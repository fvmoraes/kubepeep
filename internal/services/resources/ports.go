package resources

import (
	"context"
	"io"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

// AuthorizationChecker intentionally matches authorization.Service.Check so
// the existing generation-bound tri-state service can be injected directly.
type AuthorizationChecker interface {
	Check(context.Context, authorization.Key) authorization.Capability
}

// AuthorizationRefresher is an optional stronger capability implemented by
// authorization.Service. Long-lived streams use it to bypass the read cache
// during periodic revalidation; lightweight adapters and tests may implement
// only AuthorizationChecker and retain the conservative Check fallback.
type AuthorizationRefresher interface {
	Refresh(context.Context, authorization.Key) authorization.Capability
}

func authorizationCapability(ctx context.Context, checker AuthorizationChecker, key authorization.Key, refresh bool) authorization.Capability {
	if refresh {
		if refresher, ok := checker.(AuthorizationRefresher); ok {
			return refresher.Refresh(ctx, key)
		}
	}
	return checker.Check(ctx, key)
}

type PageRequest struct {
	Origin   Origin
	Limit    int64
	Continue string
}

// OriginLister is implemented by adapters using native Kubernetes LIST
// continuation. Items must be sorted according to the collection merge tuple.
type OriginLister[T ListItem] interface {
	ListPage(context.Context, PageRequest) (OriginPage[T], error)
}

type ResourceGetter[T DetailItem] interface {
	Get(context.Context, Origin, string) (T, error)
}

// SecretMetadataPort is deliberately separate from every generic getter. An
// implementation may issue only metadata negotiation requests and must return
// FEATURE_UNAVAILABLE instead of falling back to a typed Secret.
type SecretMetadataPort interface {
	ListSecretMetadata(context.Context, PageRequest) (OriginPage[SecretMetadataDTO], error)
	GetSecretMetadata(context.Context, string, string) (SecretMetadataDTO, error)
}

type LogTarget struct {
	Namespace string
	Pod       string
	Container string
}

type LogSourceOptions struct {
	Previous     bool
	Timestamps   bool
	TailLines    int64
	SinceSeconds *int64
	LimitBytes   int64
	Follow       bool
}

type LogPort interface {
	Open(context.Context, LogTarget, LogSourceOptions) (io.ReadCloser, error)
}

// LogTerminationProbe is optional. Implementations may report termination
// only from authoritative container state; absence, denial, or an inconclusive
// lookup must remain false so EOF is never relabeled speculatively.
type LogTerminationProbe interface {
	ContainerTerminated(context.Context, LogTarget) bool
}

type TextRedactor interface {
	Redact(string) string
}

type TextRedactorFunc func(string) string

func (function TextRedactorFunc) Redact(value string) string { return function(value) }

type PreferenceRecord struct {
	Key           string
	ValueJSON     []byte
	SchemaVersion int
}

// PreferenceRepository must replace the complete allowlisted snapshot in one
// transaction. No key/value mutation method is exposed.
type PreferenceRepository interface {
	Load(context.Context) ([]PreferenceRecord, error)
	Replace(context.Context, []PreferenceRecord) error
}

type SensitiveValueDetector interface {
	ContainsSensitiveValue(string) bool
}

type SensitiveValueDetectorFunc func(string) bool

func (function SensitiveValueDetectorFunc) ContainsSensitiveValue(value string) bool {
	return function(value)
}
