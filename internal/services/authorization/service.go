package authorization

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode"

	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	maxGenerationLength = 256
	maxRules            = 256
	maxRuleValues       = 64
	maxRuleValueLength  = 256
)

// Options configures the authorization service without introducing global
// mutable state.
type Options struct {
	TTL           time.Duration
	RulesReviewer RulesReviewer
	Now           func() time.Time
}

type cacheEntry struct {
	capability Capability
}

type reviewCall struct {
	done   chan struct{}
	result Capability
}

type revisionStamp struct {
	global     uint64
	generation uint64
	key        uint64
}

type flightKey struct {
	key      Key
	revision revisionStamp
}

// Service owns the short-lived SSAR cache and concurrent request
// deduplication. It contains no Kubernetes credentials.
type Service struct {
	reviewer      AccessReviewer
	rulesReviewer RulesReviewer
	ttl           time.Duration
	now           func() time.Time

	mu                  sync.Mutex
	cache               map[Key]cacheEntry
	inflight            map[flightKey]*reviewCall
	globalRevision      uint64
	generationRevisions map[string]uint64
	keyRevisions        map[Key]uint64
}

// New creates an authorization service. A zero TTL selects DefaultTTL; any
// explicit value outside 30-60 seconds is rejected.
func New(reviewer AccessReviewer, options Options) (*Service, error) {
	if reviewer == nil {
		return nil, validationError()
	}
	ttl := options.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}
	if ttl < MinTTL || ttl > MaxTTL {
		return nil, validationError()
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	rulesReviewer := options.RulesReviewer
	if rulesReviewer == nil {
		if candidate, ok := reviewer.(RulesReviewer); ok {
			rulesReviewer = candidate
		}
	}
	return &Service{
		reviewer:            reviewer,
		rulesReviewer:       rulesReviewer,
		ttl:                 ttl,
		now:                 now,
		cache:               make(map[Key]cacheEntry),
		inflight:            make(map[flightKey]*reviewCall),
		generationRevisions: make(map[string]uint64),
		keyRevisions:        make(map[Key]uint64),
	}, nil
}

// TTL reports the configured cache lifetime.
func (s *Service) TTL() time.Duration { return s.ttl }

// ValidateKey rejects malformed or unsafe attributes before a review is sent.
// Generation remains local and is never copied into the SSAR object.
func ValidateKey(key Key) error {
	if !safeOpaque(key.Generation, maxGenerationLength) || key.Resource == "" || key.Verb == "" {
		return validationError()
	}
	if key.Namespace != "" && len(validation.IsDNS1123Label(key.Namespace)) != 0 {
		return validationError()
	}
	if key.APIGroup != "" && len(validation.IsDNS1123Subdomain(key.APIGroup)) != 0 {
		return validationError()
	}
	if len(validation.IsDNS1123Label(key.Resource)) != 0 {
		return validationError()
	}
	if key.Subresource != "" && len(validation.IsDNS1123Label(key.Subresource)) != 0 {
		return validationError()
	}
	if len(validation.IsDNS1123Label(key.Verb)) != 0 {
		return validationError()
	}
	if key.ResourceName != "" && len(validation.IsDNS1123Subdomain(key.ResourceName)) != 0 {
		return validationError()
	}
	return nil
}

func safeOpaque(value string, maximum int) bool {
	if value == "" || len(value) > maximum || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

// Check returns a cached decision when valid, otherwise performs one SSAR.
// Concurrent misses for an identical full key share the same request.
func (s *Service) Check(ctx context.Context, key Key) Capability {
	return s.check(ctx, key, false)
}

// Refresh ignores and removes the cached value for this exact key. A currently
// running SSAR is still shared, because it is already an up-to-date remote
// review rather than a cached decision.
func (s *Service) Refresh(ctx context.Context, key Key) Capability {
	return s.check(ctx, key, true)
}

func (s *Service) check(ctx context.Context, key Key, bypassCache bool) Capability {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ValidateKey(key); err != nil {
		return capabilityFor(key, DecisionUnknown, ReasonSARUnavailable, s.now())
	}

	now := s.now()
	s.mu.Lock()
	if bypassCache {
		// Invalidate the cached decision in the same critical section that
		// joins or creates the live review. A separate lock window lets a
		// concurrent refresher delete a freshly published result and issue a
		// second SAR even though its call overlapped the first one.
		delete(s.cache, key)
	} else {
		if entry, ok := s.cache[key]; ok {
			if now.Before(entry.capability.ExpiresAt) {
				result := entry.capability
				s.mu.Unlock()
				return result
			}
			delete(s.cache, key)
		}
	}
	revision := s.revisionLocked(key)
	flight := flightKey{key: key, revision: revision}
	if call, ok := s.inflight[flight]; ok {
		s.mu.Unlock()
		select {
		case <-call.done:
			return call.result
		case <-ctx.Done():
			return capabilityFor(key, DecisionUnknown, ReasonRequestCanceled, s.now())
		}
	}
	call := &reviewCall{done: make(chan struct{})}
	s.inflight[flight] = call
	s.mu.Unlock()

	review, reviewError := s.reviewer.ReviewAccess(ctx, key)
	decision, reason := classifyReview(review, reviewError)
	result := capabilityFor(key, decision, reason, s.now().Add(s.ttl))

	s.mu.Lock()
	call.result = result
	delete(s.inflight, flight)
	if revision == s.revisionLocked(key) && !errorsCanceled(ctx, reviewError) {
		s.cache[key] = cacheEntry{capability: result}
	}
	close(call.done)
	s.mu.Unlock()
	return result
}

func errorsCanceled(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	return err != nil && ErrorCodeOf(TranslateOperationError(err)) == CodeClientCanceled
}

func classifyReview(review AccessReviewResult, err error) (Decision, ReasonCode) {
	if err != nil {
		switch ErrorCodeOf(TranslateOperationError(err)) {
		case CodeClientCanceled:
			return DecisionUnknown, ReasonRequestCanceled
		case CodeUpstreamTimeout:
			return DecisionUnknown, ReasonSARTimeout
		case CodeAuthenticationUnavailable:
			return DecisionUnknown, ReasonSARAuthenticationUnavailable
		default:
			return DecisionUnknown, ReasonSARUnavailable
		}
	}
	if !review.Complete || review.Allowed == review.Denied {
		return DecisionUnknown, ReasonSARIncomplete
	}
	if review.Allowed {
		return DecisionAllowed, ReasonSARAllowed
	}
	return DecisionDenied, ReasonSARDenied
}

func capabilityFor(key Key, decision Decision, reason ReasonCode, expiresAt time.Time) Capability {
	return Capability{
		Namespace:    key.Namespace,
		APIGroup:     key.APIGroup,
		Resource:     key.Resource,
		Subresource:  key.Subresource,
		Verb:         key.Verb,
		ResourceName: key.ResourceName,
		Decision:     decision,
		ReasonCode:   reason,
		ExpiresAt:    expiresAt.UTC(),
	}
}

// InvalidateGeneration removes all decisions for one selection generation.
// In-flight results started before invalidation are prevented from entering the
// cache even if they finish later.
func (s *Service) InvalidateGeneration(generation string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generationRevisions[generation]++
	for key := range s.cache {
		if key.Generation == generation {
			delete(s.cache, key)
		}
	}
}

// InvalidateAll clears every cached decision and fences existing in-flight
// requests from repopulating the cache.
func (s *Service) InvalidateAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.globalRevision++
	clear(s.cache)
	clear(s.generationRevisions)
	clear(s.keyRevisions)
}

func (s *Service) invalidateKey(key Key) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keyRevisions[key]++
	delete(s.cache, key)
}

func (s *Service) revisionLocked(key Key) revisionStamp {
	return revisionStamp{
		global:     s.globalRevision,
		generation: s.generationRevisions[key.Generation],
		key:        s.keyRevisions[key],
	}
}

// Revalidate applies operation policy. Mutations and upgrades always bypass
// the UI/cache decision and fail closed on unknown. Reads may continue to the
// actual operation when the review is unknown.
func (s *Service) Revalidate(ctx context.Context, key Key, kind OperationKind) (Capability, error) {
	if err := ValidateKey(key); err != nil {
		return capabilityFor(key, DecisionUnknown, ReasonSARUnavailable, s.now()), err
	}
	var capability Capability
	switch kind {
	case OperationRead:
		capability = s.Check(ctx, key)
	case OperationMutation, OperationUpgrade:
		capability = s.Refresh(ctx, key)
	default:
		return capabilityFor(key, DecisionUnknown, ReasonSARUnavailable, s.now()), validationError()
	}
	if capability.Decision == DecisionDenied {
		return capability, forbiddenError(nil)
	}
	if capability.Decision == DecisionUnknown && kind != OperationRead {
		return capability, authorizationUnavailableError(nil)
	}
	return capability, nil
}

// Guard revalidates and runs the real Kubernetes operation. A real 403 always
// wins over an earlier allowed review and invalidates that cache entry.
func (s *Service) Guard(ctx context.Context, key Key, kind OperationKind, operation Operation) (GuardResult, error) {
	if operation == nil {
		return GuardResult{}, validationError()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	capability, err := s.Revalidate(ctx, key, kind)
	result := GuardResult{Capability: capability}
	if err != nil {
		return result, err
	}

	result.Executed = true
	operationError := operation(ctx)
	if operationError == nil {
		if capability.Decision == DecisionUnknown {
			result.Capability.Decision = DecisionAllowed
			result.Capability.ReasonCode = ReasonOperationAllowed
			result.Capability.ExpiresAt = s.now().UTC()
		}
		return result, nil
	}

	publicError := TranslateOperationError(operationError)
	result.Capability.ExpiresAt = s.now().UTC()
	if publicError.Code == CodeForbidden {
		s.invalidateKey(key)
		result.Capability.Decision = DecisionDenied
		result.Capability.ReasonCode = ReasonOperationDenied
		return result, publicError
	}
	result.Capability.Decision = DecisionUnknown
	result.Capability.ReasonCode = ReasonOperationUnavailable
	return result, publicError
}

// Summary retrieves bounded SSRR hints. The result is never inserted into the
// decision cache and therefore cannot authorize Check or Guard.
func (s *Service) Summary(ctx context.Context, namespace string) RulesSummary {
	if ctx == nil {
		ctx = context.Background()
	}
	if namespace != "" && len(validation.IsDNS1123Label(namespace)) != 0 {
		return RulesSummary{Namespace: namespace, ReasonCode: ReasonSSRRUnavailable, Rules: []ResourceRuleHint{}}
	}
	if s.rulesReviewer == nil {
		return RulesSummary{Namespace: namespace, ReasonCode: ReasonSSRRUnavailable, Rules: []ResourceRuleHint{}}
	}
	review, err := s.rulesReviewer.ReviewRules(ctx, namespace)
	if err != nil {
		return RulesSummary{Namespace: namespace, ReasonCode: ReasonSSRRUnavailable, Rules: []ResourceRuleHint{}}
	}
	rules := sanitizeRuleHints(review.Rules)
	if !review.Complete {
		return RulesSummary{Namespace: namespace, Complete: false, ReasonCode: ReasonSSRRIncomplete, Rules: rules}
	}
	return RulesSummary{Namespace: namespace, Complete: true, ReasonCode: ReasonSSRRSummaryAvailable, Rules: rules}
}

func sanitizeRuleHints(source []ResourceRuleHint) []ResourceRuleHint {
	if len(source) > maxRules {
		source = source[:maxRules]
	}
	result := make([]ResourceRuleHint, 0, len(source))
	for _, rule := range source {
		result = append(result, ResourceRuleHint{
			Verbs:         sanitizeRuleValues(rule.Verbs),
			APIGroups:     sanitizeRuleValues(rule.APIGroups),
			Resources:     sanitizeRuleValues(rule.Resources),
			ResourceNames: sanitizeRuleValues(rule.ResourceNames),
		})
	}
	return result
}

func sanitizeRuleValues(source []string) []string {
	if len(source) > maxRuleValues {
		source = source[:maxRuleValues]
	}
	result := make([]string, 0, len(source))
	for _, value := range source {
		if value == "" || len(value) > maxRuleValueLength || strings.TrimSpace(value) != value {
			continue
		}
		valid := true
		for _, character := range value {
			if unicode.IsControl(character) {
				valid = false
				break
			}
		}
		if valid {
			result = append(result, value)
		}
	}
	return result
}
