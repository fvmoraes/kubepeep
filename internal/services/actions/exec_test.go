package actions

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func newTestExecManager(t *testing.T, generations *generationStub, authorizer *authorizerStub, inspector *execInspectorStub, adapter *execAdapterStub, audit AuditSink) *ExecManager {
	t.Helper()
	manager, err := newExecService(context.Background(), authorizer, generations, inspector, adapter, audit, systemClock{}, &identifierStub{}, time.Second, time.Second, time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	return manager
}

func TestExecTicketAndUpgradeRepeatExactSARAndPreserveArgv(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	authorizer := &authorizerStub{}
	inspector := &execInspectorStub{state: ExecTargetState{PodExists: true, ContainerExists: true, ContainerRunning: true}}
	adapter := &execAdapterStub{}
	audit := &auditStub{}
	manager := newTestExecManager(t, generations, authorizer, inspector, adapter, audit)
	binding := testBinding("gen_1")
	route := RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}
	argv := []string{"/bin/tool", "$(not-a-shell)", "semi;colon", ""}
	ticket, err := manager.CreateTicket(context.Background(), binding, route, testExec("gen_1", "payments", "api-abc", argv))
	if err != nil {
		t.Fatal(err)
	}
	if ticket.SessionID == "" || ticket.WebsocketURL != "/api/v1/exec/"+ticket.SessionID+"/stream" || len(ticket.Protocols) != 2 || ticket.Protocols[0] != ExecWebSocketProtocol || !strings.HasPrefix(ticket.Protocols[1], ExecTicketPrefix) {
		t.Fatalf("invalid ticket DTO: %#v", ticket)
	}
	grant, err := manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, ticket.Protocols)
	if err != nil {
		t.Fatal(err)
	}
	active, err := manager.Start(context.Background(), grant)
	if err != nil {
		t.Fatal(err)
	}
	if active.SessionID != ticket.SessionID || !active.TTY || !active.Stdin || active.Remote == nil {
		t.Fatalf("invalid active exec: %#v", active)
	}
	calls := authorizer.snapshot()
	if len(calls) != 2 {
		t.Fatalf("POST and upgrade must each revalidate: %#v", calls)
	}
	for _, call := range calls {
		key := call.key
		if key.Generation != "gen_1" || key.Namespace != "payments" || key.Resource != "pods" || key.Subresource != "exec" || key.Verb != "create" || key.ResourceName != "api-abc" || call.kind != "upgrade" {
			t.Fatalf("incorrect exec SAR: %#v", call)
		}
	}
	adapter.mu.Lock()
	command := adapter.commands[0]
	remote := adapter.remotes[0]
	adapter.mu.Unlock()
	if fmt.Sprint(command.Command) != fmt.Sprint(argv) {
		t.Fatalf("argv was transformed or shell-concatenated: got %#v want %#v", command.Command, argv)
	}
	exitCode := 0
	remote.finish(RemoteExecExit{ExitCode: &exitCode})
	terminal := <-active.Terminal
	if terminal.Type != ExecTerminalExit || terminal.ExitCode == nil || *terminal.ExitCode != 0 || terminal.Reason != ExecExitCompleted || terminal.CloseCode != 1000 {
		t.Fatalf("unexpected terminal: %#v", terminal)
	}
	for _, event := range audit.snapshot() {
		encoded := fmt.Sprintf("%#v", event)
		for _, forbidden := range argv {
			if forbidden != "" && strings.Contains(encoded, forbidden) {
				t.Fatalf("audit leaked argv %q: %s", forbidden, encoded)
			}
		}
		if strings.Contains(encoded, strings.TrimPrefix(ticket.Protocols[1], ExecTicketPrefix)) {
			t.Fatalf("audit leaked ticket: %s", encoded)
		}
	}
}

func TestExecTicketIsOneShotAndWrongTokenDoesNotConsumeIt(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	manager := newTestExecManager(t, generations, &authorizerStub{}, &execInspectorStub{state: ExecTargetState{PodExists: true, ContainerExists: true, ContainerRunning: true}}, &execAdapterStub{}, NoopAuditSink{})
	binding := testBinding("gen_1")
	route := RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}
	ticket, err := manager.CreateTicket(context.Background(), binding, route, testExec("gen_1", "payments", "api-abc", []string{"/bin/sh"}))
	if err != nil {
		t.Fatal(err)
	}
	wrong := []string{ExecWebSocketProtocol, ExecTicketPrefix + "wrong-ticket"}
	_, err = manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, wrong)
	requireCode(t, err, CodeSessionGone)
	grant, err := manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, ticket.Protocols)
	if err != nil || grant.SessionID != ticket.SessionID {
		t.Fatalf("wrong token consumed ticket: %#v err=%v", grant, err)
	}
	_, err = manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, ticket.Protocols)
	requireCode(t, err, CodeSessionGone)
}

func TestExecTicketExpiryAndGenerationBinding(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)}
	generations := &generationStub{generation: "gen_1"}
	manager, err := newExecService(context.Background(), &authorizerStub{}, generations, &execInspectorStub{state: ExecTargetState{PodExists: true, ContainerExists: true, ContainerRunning: true}}, &execAdapterStub{}, NoopAuditSink{}, clock, &identifierStub{}, 10*time.Second, time.Hour, time.Hour, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	binding := testBinding("gen_1")
	route := RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}
	ticket, err := manager.CreateTicket(context.Background(), binding, route, testExec("gen_1", "payments", "api-abc", []string{"/bin/sh"}))
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(10 * time.Second)
	_, err = manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, ticket.Protocols)
	requireCode(t, err, CodeSessionGone)

	clock.advance(time.Second)
	ticket, err = manager.CreateTicket(context.Background(), binding, route, testExec("gen_1", "payments", "api-abc", []string{"/bin/sh"}))
	if err != nil {
		t.Fatal(err)
	}
	generations.set("gen_2")
	newBinding := testBinding("gen_2")
	_, err = manager.AuthorizeUpgrade(context.Background(), newBinding, ticket.SessionID, ticket.Protocols)
	requireCode(t, err, CodeGenerationChanged)
	_, err = manager.AuthorizeUpgrade(context.Background(), newBinding, ticket.SessionID, ticket.Protocols)
	requireCode(t, err, CodeSessionGone)
}

func TestExecProtocolAndArgvValidationPrecedeSensitiveWork(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	authorizer := &authorizerStub{}
	inspector := &execInspectorStub{state: ExecTargetState{PodExists: true, ContainerExists: true, ContainerRunning: true}}
	manager := newTestExecManager(t, generations, authorizer, inspector, &execAdapterStub{}, NoopAuditSink{})
	binding := testBinding("gen_1")
	route := RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}
	invalid := testExec("gen_1", "payments", "api-abc", []string{"/bin/sh", "bad\x00argument"})
	_, err := manager.CreateTicket(context.Background(), binding, route, invalid)
	requireCode(t, err, CodeValidationFailed)
	if inspector.calls != 0 || len(authorizer.snapshot()) != 0 {
		t.Fatal("invalid argv reached target inspection or authorization")
	}

	ticket, err := manager.CreateTicket(context.Background(), binding, route, testExec("gen_1", "payments", "api-abc", []string{"/bin/sh"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, protocols := range [][]string{
		{ExecWebSocketProtocol},
		{ticket.Protocols[1], ticket.Protocols[1]},
		{ExecWebSocketProtocol, ticket.Protocols[1], "extra"},
	} {
		_, err := manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, protocols)
		requireCode(t, err, CodeValidationFailed)
	}
	if _, err := manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, ticket.Protocols); err != nil {
		t.Fatalf("invalid protocol attempt consumed valid ticket: %v", err)
	}
}

func TestExecLimitCountsReservationsAndActiveSessions(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	manager := newTestExecManager(t, generations, &authorizerStub{}, &execInspectorStub{state: ExecTargetState{PodExists: true, ContainerExists: true, ContainerRunning: true}}, &execAdapterStub{}, NoopAuditSink{})
	binding := testBinding("gen_1")
	for index := 0; index < 3; index++ {
		name := fmt.Sprintf("api-%d", index)
		route := RouteTarget{Kind: "pods", Namespace: "payments", Name: name}
		ticket, err := manager.CreateTicket(context.Background(), binding, route, testExec("gen_1", "payments", name, []string{"/bin/sh"}))
		if err != nil {
			t.Fatal(err)
		}
		_, err = manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, ticket.Protocols)
		if index < MaximumExecSessions && err != nil {
			t.Fatalf("session %d failed: %v", index, err)
		}
		if index == MaximumExecSessions {
			requireCode(t, err, CodeLimitExceeded)
		}
	}
}

func TestExecCancellationGenerationIdleAndDurationAreDeterministic(t *testing.T) {
	newManager := func(idle, duration time.Duration) (*ExecManager, *generationStub, *execAdapterStub) {
		generations := &generationStub{generation: "gen_1"}
		adapter := &execAdapterStub{}
		manager, err := newExecService(context.Background(), &authorizerStub{}, generations, &execInspectorStub{state: ExecTargetState{PodExists: true, ContainerExists: true, ContainerRunning: true}}, adapter, NoopAuditSink{}, systemClock{}, &identifierStub{}, time.Second, duration, idle, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(manager.Shutdown)
		return manager, generations, adapter
	}
	start := func(manager *ExecManager) ActiveExec {
		binding := testBinding("gen_1")
		route := RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}
		ticket, err := manager.CreateTicket(context.Background(), binding, route, testExec("gen_1", "payments", "api-abc", []string{"/bin/sh"}))
		if err != nil {
			t.Fatal(err)
		}
		grant, err := manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, ticket.Protocols)
		if err != nil {
			t.Fatal(err)
		}
		active, err := manager.Start(context.Background(), grant)
		if err != nil {
			t.Fatal(err)
		}
		return active
	}

	manager, _, _ := newManager(time.Second, time.Second)
	active := start(manager)
	if err := manager.Cancel(active.SessionID); err != nil {
		t.Fatal(err)
	}
	terminal := <-active.Terminal
	if terminal.Type != ExecTerminalExit || terminal.Reason != ExecExitCanceled || terminal.CloseCode != 1000 {
		t.Fatalf("cancel mapping: %#v", terminal)
	}

	manager, generations, _ := newManager(time.Second, time.Second)
	active = start(manager)
	generations.set("gen_2")
	manager.OnGeneration("gen_2")
	terminal = <-active.Terminal
	if terminal.Type != ExecTerminalError || terminal.Code != CodeGenerationChanged || terminal.CloseCode != 1001 {
		t.Fatalf("generation mapping: %#v", terminal)
	}

	manager, _, _ = newManager(20*time.Millisecond, time.Second)
	active = start(manager)
	terminal = <-active.Terminal
	if terminal.Code != CodeExecIdleTimeout || terminal.CloseCode != 1001 {
		t.Fatalf("idle mapping: %#v", terminal)
	}

	manager, _, _ = newManager(time.Second, 20*time.Millisecond)
	active = start(manager)
	terminal = <-active.Terminal
	if terminal.Code != CodeExecDurationLimit || terminal.CloseCode != 1001 {
		t.Fatalf("duration mapping: %#v", terminal)
	}
}

func TestExecRemoteExitMappings(t *testing.T) {
	tests := []struct {
		name      string
		exit      RemoteExecExit
		typeValue ExecTerminalType
		reason    ExecExitReason
		code      ErrorCode
		closeCode int
	}{
		{name: "nonzero", exit: RemoteExecExit{ExitCode: intPointer(7)}, typeValue: ExecTerminalExit, reason: ExecExitRemoteError, closeCode: 1000},
		{name: "signal", exit: RemoteExecExit{Signaled: true}, typeValue: ExecTerminalExit, reason: ExecExitSignal, closeCode: 1000},
		{name: "target gone", exit: RemoteExecExit{Err: ErrExecTargetGone}, typeValue: ExecTerminalError, code: CodeExecTargetGone, closeCode: 1008},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			terminal := terminalForRemoteExit(test.exit)
			if terminal.Type != test.typeValue || terminal.Reason != test.reason || terminal.Code != test.code || terminal.CloseCode != test.closeCode {
				t.Fatalf("unexpected terminal: %#v", terminal)
			}
		})
	}
}

func TestExecRejectsMissingOrStoppedContainerBeforeSAR(t *testing.T) {
	for _, state := range []ExecTargetState{
		{PodExists: false},
		{PodExists: true, ContainerExists: false},
		{PodExists: true, ContainerExists: true, ContainerRunning: false},
	} {
		authorizer := &authorizerStub{}
		manager := newTestExecManager(t, &generationStub{generation: "gen_1"}, authorizer, &execInspectorStub{state: state}, &execAdapterStub{}, NoopAuditSink{})
		_, err := manager.CreateTicket(context.Background(), testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}, testExec("gen_1", "payments", "api-abc", []string{"/bin/sh"}))
		if err == nil {
			t.Fatalf("state %#v was accepted", state)
		}
		if len(authorizer.snapshot()) != 0 {
			t.Fatalf("invalid target reached SAR: %#v", state)
		}
	}
}

func intPointer(value int) *int { return &value }
