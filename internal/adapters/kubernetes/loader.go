package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const defaultMaxTrackedFileSize int64 = 16 << 20

// Source identifies the selected precedence branch. It is descriptive only;
// the logical client-cache identity is paths plus context.
type Source string

const (
	SourceExplicit    Source = "explicit"
	SourceEnvironment Source = "environment"
	SourceProfile     Source = "profile"
	SourceRecommended Source = "recommended"
)

// ProfileReference is the only kubeconfig information intended for durable
// storage: the ordered paths and the selected context name.
type ProfileReference struct {
	Paths   []string
	Context string
}

// Descriptor is credential-free. Paths remain operational metadata and must
// still be omitted or display-sanitized by HTTP DTOs and logs.
type Descriptor struct {
	Paths   []string
	Context string
	Source  Source
}

// ContextDescriptor is the credential-free portion of one kubeconfig context.
// User/auth-info names and cluster server addresses are deliberately omitted.
type ContextDescriptor struct {
	Name    string
	Cluster string
}

type ExecPluginDiagnostic struct {
	Configured        bool
	CommandAvailable  bool
	NonInteractive    bool
	VersionCompatible bool
}

// CacheKey returns an opaque logical key over ordered normalized paths and
// context. Source and transient fingerprints are intentionally excluded.
func (d Descriptor) CacheKey() string {
	hash := sha256.New()
	writeKeyPart(hash, "kubepeep-client-v1")
	for _, path := range d.Paths {
		writeKeyPart(hash, path)
	}
	writeKeyPart(hash, d.Context)
	return hex.EncodeToString(hash.Sum(nil))
}

type keyWriter interface {
	Write([]byte) (int, error)
}

func writeKeyPart(writer keyWriter, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write([]byte(value))
}

// ResolveRequest carries startup precedence inputs. A non-nil ExplicitPath or
// ExplicitContext means the corresponding flag was present, including an
// invalid empty value.
type ResolveRequest struct {
	ExplicitPath    *string
	ExplicitContext *string
	Persisted       *ProfileReference
	FirstReconcile  bool
	// ProfileOnly is used after an explicit local profile selection. It keeps
	// ambient KUBECONFIG from replacing that profile's ordered source paths.
	ProfileOnly bool
}

// LoaderOptions make environment and platform defaults testable without
// mutable globals.
type LoaderOptions struct {
	LookupEnv          func(string) (string, bool)
	RecommendedPath    func() string
	MaxTrackedFileSize int64
}

// Loader resolves kubeconfigs with official client-go merging semantics.
type Loader struct {
	lookupEnv       func(string) (string, bool)
	recommendedPath func() string
	maxFileSize     int64
}

func NewLoader(options LoaderOptions) *Loader {
	lookup := options.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	recommended := options.RecommendedPath
	if recommended == nil {
		recommended = func() string { return clientcmd.RecommendedHomeFile }
	}
	maxFileSize := options.MaxTrackedFileSize
	if maxFileSize <= 0 {
		maxFileSize = defaultMaxTrackedFileSize
	}
	return &Loader{lookupEnv: lookup, recommendedPath: recommended, maxFileSize: maxFileSize}
}

// Resolution keeps parsed credentials and rest.Config exclusively in memory.
// Callers receive clients through ClientFactory rather than serializing it.
type Resolution struct {
	descriptor  Descriptor
	raw         *clientcmdapi.Config
	restConfig  *rest.Config
	tracked     []trackedFile
	fingerprint Fingerprint
	maxFileSize int64
}

func (r *Resolution) Descriptor() Descriptor {
	if r == nil {
		return Descriptor{}
	}
	descriptor := r.descriptor
	descriptor.Paths = append([]string(nil), descriptor.Paths...)
	return descriptor
}

// Contexts returns a deterministic, credential-free view of the contexts in
// the merged kubeconfig. It is safe to pass through an application DTO.
func (r *Resolution) Contexts() []ContextDescriptor {
	if r == nil || r.raw == nil {
		return []ContextDescriptor{}
	}
	names := make([]string, 0, len(r.raw.Contexts))
	for name := range r.raw.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]ContextDescriptor, 0, len(names))
	for _, name := range names {
		cluster := ""
		if configured := r.raw.Contexts[name]; configured != nil {
			cluster = configured.Cluster
		}
		result = append(result, ContextDescriptor{Name: name, Cluster: cluster})
	}
	return result
}

// SelectedContext returns the selected context and its public cluster name.
// The boolean is false when the resolution intentionally has no selection.
func (r *Resolution) SelectedContext() (ContextDescriptor, bool) {
	if r == nil || r.raw == nil || r.descriptor.Context == "" {
		return ContextDescriptor{}, false
	}
	configured, exists := r.raw.Contexts[r.descriptor.Context]
	if !exists || configured == nil {
		return ContextDescriptor{}, false
	}
	return ContextDescriptor{Name: r.descriptor.Context, Cluster: configured.Cluster}, true
}

// ExecPlugin reports only fixed booleans about the selected auth-info. The
// command, args, environment, install hint and plugin output are never exposed.
func (r *Resolution) ExecPlugin() ExecPluginDiagnostic {
	if r == nil || r.raw == nil || r.descriptor.Context == "" {
		return ExecPluginDiagnostic{}
	}
	configuredContext := r.raw.Contexts[r.descriptor.Context]
	if configuredContext == nil {
		return ExecPluginDiagnostic{}
	}
	authInfo := r.raw.AuthInfos[configuredContext.AuthInfo]
	if authInfo == nil || authInfo.Exec == nil {
		return ExecPluginDiagnostic{}
	}
	plugin := authInfo.Exec
	_, commandErr := exec.LookPath(plugin.Command)
	return ExecPluginDiagnostic{
		Configured:        true,
		CommandAvailable:  plugin.Command != "" && commandErr == nil,
		NonInteractive:    plugin.InteractiveMode != clientcmdapi.AlwaysExecInteractiveMode,
		VersionCompatible: plugin.APIVersion == "client.authentication.k8s.io/v1" || plugin.APIVersion == "client.authentication.k8s.io/v1beta1",
	}
}

func (r *Resolution) Fingerprint() Fingerprint {
	if r == nil {
		return Fingerprint{}
	}
	return r.fingerprint
}

// CurrentFingerprint rereads every source and referenced credential file. It
// allows a cache to reject a stale resolution before activating it.
func (r *Resolution) CurrentFingerprint(ctx context.Context) (Fingerprint, error) {
	if r == nil {
		return Fingerprint{}, safeError(CodeKubeconfigInvalid, "The kubeconfig could not be interpreted safely.", false)
	}
	return fingerprintFiles(ctx, r.tracked, r.maxFileSize)
}

func (r *Resolution) configCopy() (*rest.Config, error) {
	if r == nil || r.restConfig == nil {
		return nil, safeError(CodeContextRequired, "A Kubernetes context must be selected.", false)
	}
	copy := rest.CopyConfig(r.restConfig)
	if !impersonationIsEmpty(copy.Impersonate) {
		return nil, safeError(CodeKubeconfigInvalid, "The kubeconfig requested an unsupported capability.", false)
	}
	return copy, nil
}

// Resolve applies the canonical source and context precedence. Once a branch
// is selected, any error is returned and lower-precedence branches are never
// attempted.
func (loader *Loader) Resolve(ctx context.Context, request ResolveRequest) (*Resolution, error) {
	if ctx == nil {
		return nil, safeError(CodeKubeconfigInvalid, "The kubeconfig could not be interpreted safely.", false)
	}
	if err := ctx.Err(); err != nil {
		return nil, SanitizeError(err)
	}

	paths, source, explicitFile, err := loader.selectSource(request)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizePaths(paths)
	if err != nil {
		return nil, err
	}
	sources := make([]trackedFile, 0, len(normalized))
	for position, path := range normalized {
		sources = append(sources, trackedFile{
			label:  fmt.Sprintf("source:%08d", position),
			path:   path,
			source: true,
		})
	}
	if _, err := fingerprintFiles(ctx, sources, loader.maxFileSize); err != nil {
		return nil, err
	}
	rules := &clientcmd.ClientConfigLoadingRules{
		Precedence:        append([]string(nil), normalized...),
		DoNotResolvePaths: false,
		WarnIfAllMissing:  false,
	}
	if explicitFile {
		rules.ExplicitPath = normalized[0]
		rules.Precedence = nil
	}
	raw, rawError := rules.Load()
	if rawError != nil || raw == nil {
		return nil, safeError(CodeKubeconfigInvalid, "The kubeconfig could not be interpreted safely.", false)
	}
	if err := ctx.Err(); err != nil {
		return nil, SanitizeError(err)
	}

	contextName, selected, err := selectContext(raw, request)
	if err != nil {
		return nil, err
	}
	descriptor := Descriptor{Paths: normalized, Context: contextName, Source: source}
	tracked := referencedFiles(normalized, raw)
	fingerprint, err := fingerprintFiles(ctx, tracked, loader.maxFileSize)
	if err != nil {
		return nil, err
	}

	resolution := &Resolution{
		descriptor:  descriptor,
		raw:         raw,
		tracked:     tracked,
		fingerprint: fingerprint,
		maxFileSize: loader.maxFileSize,
	}
	if !selected {
		return resolution, nil
	}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}
	clientConfig := clientcmd.NewNonInteractiveClientConfig(*raw, contextName, overrides, rules)
	restConfig, configError := clientConfig.ClientConfig()
	if configError != nil || restConfig == nil {
		return nil, safeError(CodeKubeconfigInvalid, "The selected Kubernetes context is invalid.", false)
	}
	if !impersonationIsEmpty(restConfig.Impersonate) {
		return nil, safeError(CodeKubeconfigInvalid, "The kubeconfig requested an unsupported capability.", false)
	}
	restConfig.Impersonate = rest.ImpersonationConfig{}
	resolution.restConfig = restConfig
	return resolution, nil
}

func (loader *Loader) selectSource(request ResolveRequest) ([]string, Source, bool, error) {
	if request.ProfileOnly {
		if request.Persisted == nil || len(request.Persisted.Paths) == 0 {
			return nil, "", false, safeError(CodeKubeconfigInvalid, "The persisted kubeconfig source is invalid.", false)
		}
		return request.Persisted.Paths, SourceProfile, false, nil
	}
	if request.ExplicitPath != nil {
		if *request.ExplicitPath == "" {
			return nil, "", false, safeError(CodeKubeconfigInvalid, "The explicit kubeconfig path is invalid.", false)
		}
		paths := filepath.SplitList(*request.ExplicitPath)
		if len(paths) == 0 {
			return nil, "", false, safeError(CodeKubeconfigInvalid, "The explicit kubeconfig path is invalid.", false)
		}
		return paths, SourceExplicit, len(paths) == 1, nil
	}
	if value, present := loader.lookupEnv(clientcmd.RecommendedConfigPathEnvVar); present && value != "" {
		paths := filepath.SplitList(value)
		if len(paths) == 0 {
			return nil, "", false, safeError(CodeKubeconfigInvalid, "The KUBECONFIG source is invalid.", false)
		}
		return paths, SourceEnvironment, false, nil
	}
	if request.Persisted != nil {
		if len(request.Persisted.Paths) == 0 {
			return nil, "", false, safeError(CodeKubeconfigInvalid, "The persisted kubeconfig source is invalid.", false)
		}
		return request.Persisted.Paths, SourceProfile, false, nil
	}
	recommended := loader.recommendedPath()
	if recommended == "" {
		return nil, "", false, safeError(CodeKubeconfigNotFound, "No kubeconfig is available.", false)
	}
	return []string{recommended}, SourceRecommended, true, nil
}

func normalizePaths(paths []string) ([]string, error) {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(filepath.Clean(path))
		if err != nil {
			return nil, safeError(CodeKubeconfigInvalid, "A kubeconfig path is invalid.", false)
		}
		if _, duplicate := seen[absolute]; duplicate {
			continue
		}
		seen[absolute] = struct{}{}
		result = append(result, absolute)
	}
	if len(result) == 0 {
		return nil, safeError(CodeKubeconfigInvalid, "The kubeconfig source contains no usable paths.", false)
	}
	return result, nil
}

func selectContext(raw *clientcmdapi.Config, request ResolveRequest) (string, bool, error) {
	var contextName string
	var explicitlySelected bool
	switch {
	case request.ExplicitContext != nil:
		contextName = *request.ExplicitContext
		explicitlySelected = true
	case request.Persisted != nil && request.Persisted.Context != "":
		contextName = request.Persisted.Context
		explicitlySelected = true
	case request.FirstReconcile:
		contextName = raw.CurrentContext
	}
	if contextName == "" {
		if explicitlySelected {
			return "", false, safeError(CodeContextNotFound, "The selected Kubernetes context does not exist.", false)
		}
		return "", false, nil
	}
	if !validContextName(contextName) {
		return "", false, safeError(CodeContextNotFound, "The selected Kubernetes context does not exist.", false)
	}
	if _, exists := raw.Contexts[contextName]; !exists {
		return "", false, safeError(CodeContextNotFound, "The selected Kubernetes context does not exist.", false)
	}
	return contextName, true, nil
}

func validContextName(value string) bool {
	if len(value) == 0 || len(value) > 1024 || !utf8.ValidString(value) {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) < 0
}

func impersonationIsEmpty(config rest.ImpersonationConfig) bool {
	return config.UserName == "" && config.UID == "" && len(config.Groups) == 0 && len(config.Extra) == 0
}
