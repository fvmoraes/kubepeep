package actions

import (
	"context"
	"testing"
)

func TestExecReleaseUpgradeRemovesConsumedTicketReservation(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	manager := newTestExecManager(t, generations, &authorizerStub{}, &execInspectorStub{state: ExecTargetState{PodExists: true, ContainerExists: true, ContainerRunning: true}}, &execAdapterStub{}, NoopAuditSink{})
	binding := testBinding("gen_1")
	ticket, err := manager.CreateTicket(context.Background(), binding, RouteTarget{Kind: "pods", Namespace: "payments", Name: "api"}, testExec("gen_1", "payments", "api", []string{"/bin/sh"}))
	if err != nil {
		t.Fatal(err)
	}
	grant, err := manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, ticket.Protocols)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ReleaseUpgrade(grant); err != nil {
		t.Fatal(err)
	}
	_, err = manager.Start(context.Background(), grant)
	requireCode(t, err, CodeSessionGone)
	if err := manager.ReleaseUpgrade(grant); ErrorCodeOf(err) != CodeSessionGone {
		t.Fatalf("second release = %v", err)
	}
}

func TestExecAbortReasonsOwnExactTerminalCodes(t *testing.T) {
	for _, test := range []struct {
		reason    ExecAbortReason
		code      ErrorCode
		retryable bool
		closeCode int
	}{
		{ExecAbortProtocolViolation, CodeProtocolViolation, false, 1008},
		{ExecAbortBackpressure, CodeLimitExceeded, true, 1008},
		{ExecAbortMessageTooLarge, CodeLimitExceeded, false, 1009},
		{ExecAbortInternal, CodeInternal, false, 1011},
	} {
		t.Run(string(test.reason), func(t *testing.T) {
			generations := &generationStub{generation: "gen_1"}
			manager := newTestExecManager(t, generations, &authorizerStub{}, &execInspectorStub{state: ExecTargetState{PodExists: true, ContainerExists: true, ContainerRunning: true}}, &execAdapterStub{}, NoopAuditSink{})
			binding := testBinding("gen_1")
			ticket, err := manager.CreateTicket(context.Background(), binding, RouteTarget{Kind: "pods", Namespace: "payments", Name: "api"}, testExec("gen_1", "payments", "api", []string{"/bin/sh"}))
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
			if err := manager.Abort(active.SessionID, test.reason); err != nil {
				t.Fatal(err)
			}
			terminal := <-active.Terminal
			if terminal.Type != ExecTerminalError || terminal.Code != test.code || terminal.Retryable != test.retryable || terminal.CloseCode != test.closeCode {
				t.Fatalf("terminal = %#v", terminal)
			}
		})
	}
}
