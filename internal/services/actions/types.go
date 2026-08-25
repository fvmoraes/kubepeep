// Package actions implements the authorization and lifecycle boundary for
// Kubernetes mutations and interactive sessions.
package actions

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

const (
	DefaultActionTimeout           = 30 * time.Second
	DefaultIdempotencyTTL          = 10 * time.Minute
	DefaultPortForwardDuration     = 8 * time.Hour
	DefaultPortForwardRetention    = 10 * time.Minute
	DefaultPortForwardSetup        = 15 * time.Second
	DefaultExecTicketTTL           = 10 * time.Second
	DefaultExecDuration            = 4 * time.Hour
	DefaultExecIdleTimeout         = 30 * time.Minute
	DefaultExecSetupTimeout        = 15 * time.Second
	MaximumPortForwardSessions     = 8
	MaximumExecSessions            = 2
	MaximumExecArguments           = 64
	MaximumExecArgumentBytes       = 4 * 1024
	MaximumExecCommandBytes        = 32 * 1024
	MaximumExecDataMessageBytes    = 64 * 1024
	MaximumExecControlMessageBytes = 4 * 1024
)

type Action string

const (
	ActionRestart     Action = "restart"
	ActionScale       Action = "scale"
	ActionDeletePod   Action = "deletePod"
	ActionPortForward Action = "portForward"
	ActionExec        Action = "exec"
)

type ConsequenceCode string

const (
	ConsequenceRecreateWorkloadPods   ConsequenceCode = "RECREATE_WORKLOAD_PODS"
	ConsequenceChangeReplicaCount     ConsequenceCode = "CHANGE_REPLICA_COUNT"
	ConsequenceDeletePod              ConsequenceCode = "DELETE_POD"
	ConsequenceExposePodPortLocally   ConsequenceCode = "EXPOSE_POD_PORT_LOCALLY"
	ConsequenceOpenInteractiveProcess ConsequenceCode = "OPEN_INTERACTIVE_PROCESS"
)

// ActionTargetDTO is repeated in every action request and must match both the
// route and the active selection. It deliberately contains no Kubernetes
// object payload.
type ActionTargetDTO struct {
	ClusterProfileID int64  `json:"clusterProfileId"`
	Context          string `json:"context"`
	Namespace        string `json:"namespace"`
	Kind             string `json:"kind"`
	Name             string `json:"name"`
}

// RouteTarget is populated only from the already-decoded route. Kind is the
// canonical lowercase plural used by the API path.
type RouteTarget struct {
	Kind      string
	Namespace string
	Name      string
}

type Confirmation struct {
	Confirmed          bool            `json:"confirmed"`
	Action             Action          `json:"action"`
	ConsequenceCode    ConsequenceCode `json:"consequenceCode"`
	Target             ActionTargetDTO `json:"target"`
	ExpectedGeneration string          `json:"expectedGeneration"`
}

type RestartRequest struct {
	Confirmation
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}

type ScaleRequest struct {
	Replicas int64 `json:"replicas"`
	Confirmation
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}

type PodDeleteRequest struct {
	Confirmation
	ExpectedUID             string `json:"expectedUid"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}

type PortForwardCreateRequest struct {
	RemotePort int  `json:"remotePort"`
	LocalPort  *int `json:"localPort"`
	Confirmation
}

type PortForwardDeleteRequest struct {
	Confirmed          bool   `json:"confirmed"`
	ExpectedGeneration string `json:"expectedGeneration"`
}

type ExecInit struct {
	Container string   `json:"container"`
	Command   []string `json:"command"`
	TTY       bool     `json:"tty"`
	Stdin     bool     `json:"stdin"`
	Confirmation
}

type MutationTarget struct {
	ClusterProfileID int64
	Context          string
	Generation       string
	Namespace        string
	Kind             string
	Name             string
}

type RestartDeploymentCommand struct {
	Target                  MutationTarget
	ExpectedResourceVersion string
	RestartedAt             time.Time
}

type ScaleCommand struct {
	Target                  MutationTarget
	Replicas                int32
	ExpectedResourceVersion string
}

type DeletePodCommand struct {
	Target                  MutationTarget
	ExpectedUID             string
	ExpectedResourceVersion string
}

type MutationResult struct {
	ResourceVersion string
}

type ActionAcceptedDTO struct {
	Accepted        bool            `json:"accepted"`
	Action          Action          `json:"action"`
	Target          ActionTargetDTO `json:"target"`
	Generation      string          `json:"generation"`
	ResourceVersion *string         `json:"resourceVersion"`
}

type ScaleResultDTO struct {
	Accepted        bool            `json:"accepted"`
	Action          Action          `json:"action"`
	Target          ActionTargetDTO `json:"target"`
	Generation      string          `json:"generation"`
	ResourceVersion *string         `json:"resourceVersion"`
	Replicas        int32           `json:"replicas"`
}

// KubernetesActions is intentionally narrower than a generic Kubernetes
// client: an adapter cannot receive or apply an arbitrary patch from this
// package.
type KubernetesActions interface {
	RestartDeployment(context.Context, RestartDeploymentCommand) (MutationResult, error)
	UpdateScale(context.Context, ScaleCommand) (MutationResult, error)
	DeletePod(context.Context, DeletePodCommand) (MutationResult, error)
}

type AuthorizationService interface {
	Revalidate(context.Context, authorization.Key, authorization.OperationKind) (authorization.Capability, error)
	Guard(context.Context, authorization.Key, authorization.OperationKind, authorization.Operation) (authorization.GuardResult, error)
}

type GenerationReader interface {
	CurrentGeneration() string
}

type ActionService interface {
	Restart(context.Context, namespaces.SelectionBinding, RouteTarget, string, RestartRequest) (ActionAcceptedDTO, bool, error)
	Scale(context.Context, namespaces.SelectionBinding, RouteTarget, ScaleRequest) (ScaleResultDTO, error)
	DeletePod(context.Context, namespaces.SelectionBinding, RouteTarget, PodDeleteRequest) (ActionAcceptedDTO, error)
	OnGeneration(string)
	Shutdown()
}

type PortForwardCommand struct {
	Target     MutationTarget
	RemotePort int
}

// PortForwardHandle owns only the upstream transport. The service owns the
// loopback listener and closes both sides on every terminal path.
type PortForwardHandle interface {
	Wait() error
	Close() error
}

type PortForwardAdapter interface {
	Start(setup context.Context, lifetime context.Context, command PortForwardCommand, listener net.Listener) (PortForwardHandle, error)
}

type LoopbackBinder interface {
	Listen(context.Context, int) (net.Listener, error)
}

type PortForwardStatus string

const (
	PortForwardActive  PortForwardStatus = "active"
	PortForwardClosed  PortForwardStatus = "closed"
	PortForwardExpired PortForwardStatus = "expired"
	PortForwardPodGone PortForwardStatus = "podGone"
	PortForwardFailed  PortForwardStatus = "failed"
)

type PortForwardDTO struct {
	ID               string            `json:"id"`
	ClusterProfileID int64             `json:"clusterProfileId"`
	Context          string            `json:"context"`
	Generation       string            `json:"generation"`
	Namespace        string            `json:"namespace"`
	Pod              string            `json:"pod"`
	RemotePort       int               `json:"remotePort"`
	LocalAddress     string            `json:"localAddress"`
	LocalPort        int               `json:"localPort"`
	Status           PortForwardStatus `json:"status"`
	CreatedAt        time.Time         `json:"createdAt"`
	ExpiresAt        time.Time         `json:"expiresAt"`
	EndedAt          *time.Time        `json:"endedAt"`
	EndReason        *string           `json:"endReason"`
}

type PortForwardService interface {
	Create(context.Context, namespaces.SelectionBinding, RouteTarget, string, PortForwardCreateRequest) (PortForwardDTO, bool, error)
	List(namespaces.SelectionBinding) ([]PortForwardDTO, error)
	Close(context.Context, namespaces.SelectionBinding, string, PortForwardDeleteRequest) error
	OnGeneration(string)
	Shutdown()
}

type ExecTargetState struct {
	PodExists        bool
	ContainerExists  bool
	ContainerRunning bool
}

type ExecTargetInspector interface {
	InspectExecTarget(context.Context, MutationTarget, string) (ExecTargetState, error)
}

type ExecCommand struct {
	Target    MutationTarget
	Container string
	Command   []string
	TTY       bool
	Stdin     bool
}

type RemoteExecExit struct {
	ExitCode *int
	Signaled bool
	Err      error
}

// RemoteExec exposes transient streams to the WebSocket bridge. The actions
// service never reads, stores, hashes, or logs their contents.
type RemoteExec interface {
	ExecStreams
	Wait() RemoteExecExit
	Close() error
}

type ExecStreams interface {
	Stdin() io.WriteCloser
	Stdout() io.Reader
	Stderr() io.Reader
	Resize(context.Context, uint16, uint16) error
}

type ExecAdapter interface {
	Start(setup context.Context, lifetime context.Context, command ExecCommand) (RemoteExec, error)
}

type ExecTicketDTO struct {
	SessionID    string    `json:"sessionId"`
	WebsocketURL string    `json:"websocketUrl"`
	Protocols    []string  `json:"protocols"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type ExecGrant struct {
	SessionID  string
	Generation string
	leaseID    string
}

type ExecTerminalType string

const (
	ExecTerminalExit  ExecTerminalType = "exit"
	ExecTerminalError ExecTerminalType = "error"
)

type ExecExitReason string

const (
	ExecExitCompleted   ExecExitReason = "completed"
	ExecExitRemoteError ExecExitReason = "remote_error"
	ExecExitSignal      ExecExitReason = "signal"
	ExecExitCanceled    ExecExitReason = "canceled"
)

type ExecTerminal struct {
	Type      ExecTerminalType
	ExitCode  *int
	Reason    ExecExitReason
	Code      ErrorCode
	Message   string
	Retryable bool
	CloseCode int
}

type ActiveExec struct {
	SessionID  string
	Generation string
	TTY        bool
	Stdin      bool
	Remote     ExecStreams
	Terminal   <-chan ExecTerminal
}

type ExecService interface {
	CreateTicket(context.Context, namespaces.SelectionBinding, RouteTarget, ExecInit) (ExecTicketDTO, error)
	AuthorizeUpgrade(context.Context, namespaces.SelectionBinding, string, []string) (ExecGrant, error)
	Start(context.Context, ExecGrant) (ActiveExec, error)
	Touch(string) error
	Cancel(string) error
	OnGeneration(string)
	Shutdown()
}

type Clock interface {
	Now() time.Time
}

type IdentifierSource interface {
	NewID(string) (string, error)
	NewToken() (string, error)
}
