package resources

import (
	"context"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

var collectionGVR = map[Collection]Origin{
	CollectionPods: {Version: "v1", Resource: "pods"}, CollectionEvents: {Version: "v1", Resource: "events"}, CollectionServices: {Version: "v1", Resource: "services"},
	CollectionIngresses: {APIGroup: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, CollectionEndpointSlices: {APIGroup: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
	CollectionConfigMaps: {Version: "v1", Resource: "configmaps"}, CollectionSecrets: {Version: "v1", Resource: "secrets"},
	CollectionLeases:                 {APIGroup: "coordination.k8s.io", Version: "v1", Resource: "leases"},
	CollectionPersistentVolumeClaims: {Version: "v1", Resource: "persistentvolumeclaims"},
	CollectionRoles:                  {APIGroup: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
	CollectionRoleBindings:           {APIGroup: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
	CollectionNetworkPolicies:        {APIGroup: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
	CollectionEndpoints:              {Version: "v1", Resource: "endpoints"},
	CollectionServiceAccounts:        {Version: "v1", Resource: "serviceaccounts"},
	CollectionResourceQuotas:         {Version: "v1", Resource: "resourcequotas"},
	CollectionLimitRanges:            {Version: "v1", Resource: "limitranges"},
	CollectionHPAs:                   {APIGroup: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"},
	CollectionPDBs:                   {APIGroup: "policy", Version: "v1", Resource: "poddisruptionbudgets"},
}
var workloadGVR = map[WorkloadKind]Origin{WorkloadDeployments: {APIGroup: "apps", Version: "v1", Resource: "deployments"}, WorkloadStatefulSets: {APIGroup: "apps", Version: "v1", Resource: "statefulsets"}, WorkloadDaemonSets: {APIGroup: "apps", Version: "v1", Resource: "daemonsets"}, WorkloadJobs: {APIGroup: "batch", Version: "v1", Resource: "jobs"}, WorkloadCronJobs: {APIGroup: "batch", Version: "v1", Resource: "cronjobs"}, WorkloadReplicaSets: {APIGroup: "apps", Version: "v1", Resource: "replicasets"}}

func OriginsFor(collection Collection, namespaces []string, kinds []WorkloadKind) ([]Origin, error) {
	namespaces, err := canonicalStrings(namespaces, MaximumNamespaces, nil, "namespace")
	if err != nil {
		return nil, err
	}
	return originsFor(collection, namespaces, kinds)
}

// GlobalOriginsFor returns cluster-wide LIST origins. An empty namespace is
// Kubernetes' native all-namespaces selector; callers must prove the matching
// cluster-wide list capability before passing these origins to Collect.
func GlobalOriginsFor(collection Collection, kinds []WorkloadKind) ([]Origin, error) {
	return originsFor(collection, []string{""}, kinds)
}

func originsFor(collection Collection, namespaces []string, kinds []WorkloadKind) ([]Origin, error) {
	if collection == CollectionWorkloads {
		if len(kinds) == 0 {
			kinds = append([]WorkloadKind(nil), canonicalWorkloadKinds...)
		}
		canonical, err := canonicalKinds(kinds)
		if err != nil {
			return nil, err
		}
		kinds = canonical
		result := make([]Origin, 0, len(namespaces)*len(kinds))
		for _, kind := range kinds {
			base := workloadGVR[kind]
			for _, namespace := range namespaces {
				origin := base
				origin.Namespace = namespace
				result = append(result, origin)
			}
		}
		return canonicalOrigins(result), nil
	}
	base, ok := collectionGVR[collection]
	if resource, dynamic := ParseDynamicCollection(collection); dynamic && resource.Namespaced {
		base, ok = Origin{APIGroup: resource.Group, Version: resource.Version, Resource: resource.Resource}, true
	}
	if !ok {
		return nil, validationError("collection is not supported")
	}
	result := make([]Origin, 0, len(namespaces))
	for _, namespace := range namespaces {
		origin := base
		origin.Namespace = namespace
		result = append(result, origin)
	}
	return canonicalOrigins(result), nil
}

type GetRequest[T DetailItem] struct {
	Selection          Selection
	Origin             Origin
	Name               string
	Getter             ResourceGetter[T]
	Authorizer         AuthorizationChecker
	Timeout            time.Duration
	ReadThroughUnknown bool
}

func GetAuthorized[T DetailItem](ctx context.Context, request GetRequest[T]) (T, error) {
	var zero T
	if request.Getter == nil || request.Authorizer == nil {
		return zero, domainError(CodeFeatureUnavailable, "The resource reader is unavailable.", nil)
	}
	if request.Selection.Generation == "" || request.Origin.Namespace == "" || request.Origin.Version == "" || request.Origin.Resource == "" || request.Name == "" {
		return zero, validationError("resource target is incomplete")
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	capability := request.Authorizer.Check(requestContext, authorization.Key{Generation: request.Selection.Generation, Namespace: request.Origin.Namespace, APIGroup: request.Origin.APIGroup, Resource: request.Origin.Resource, Verb: "get", ResourceName: request.Name})
	switch capability.Decision {
	case authorization.DecisionDenied:
		return zero, domainError(CodeForbidden, "Access to this resource was denied.", nil)
	case authorization.DecisionUnknown:
		if !request.ReadThroughUnknown {
			failure := authorization.ReviewFailure(capability)
			return zero, domainError(ErrorCode(failure.Code), failure.Message, nil)
		}
	}
	value, err := RetryRead(requestContext, func(ctx context.Context) (T, error) { return request.Getter.Get(ctx, request.Origin, request.Name) })
	if err != nil {
		return zero, sanitizePortError(err)
	}
	return value, nil
}
