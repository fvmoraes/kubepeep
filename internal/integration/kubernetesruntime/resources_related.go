package kubernetesruntime

import (
	"context"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

const (
	relatedListPageSize       = int64(500)
	maximumWorkloadRelated    = 256
	maximumRelatedScanItems   = 5000
	maximumCronJobHistoryScan = 10000
)

func (backend *ResourceBackend) canListNamespace(ctx context.Context, binding namespaces.SelectionBinding, namespace, apiGroup, resource string) bool {
	if backend == nil || backend.authorizer == nil {
		return false
	}
	capability := backend.authorizer.Check(ctx, authorization.Key{Generation: binding.Generation, Namespace: namespace, APIGroup: apiGroup, Resource: resource, Verb: "list"})
	return capability.Decision == authorization.DecisionAllowed
}

// cronJobHistory scans a bounded namespace-wide Job collection. A false
// complete result is security/correctness evidence: callers must return
// Unknown instead of inferring absence of a newer owned failure.
func (backend *ResourceBackend) cronJobHistory(ctx context.Context, binding namespaces.SelectionBinding, client kubeclient.Interface, namespace string) ([]batchv1.Job, bool) {
	if client == nil || !backend.canListNamespace(ctx, binding, namespace, "batch", "jobs") {
		return nil, false
	}
	result := make([]batchv1.Job, 0)
	continueToken := ""
	for {
		list, err := client.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{Limit: relatedListPageSize, Continue: continueToken})
		if err != nil {
			return nil, false
		}
		if len(result)+len(list.Items) > maximumCronJobHistoryScan {
			return nil, false
		}
		result = append(result, list.Items...)
		continueToken = list.Continue
		if continueToken == "" {
			return result, true
		}
	}
}

func (backend *ResourceBackend) relatedPodEvents(ctx context.Context, binding namespaces.SelectionBinding, client kubeclient.Interface, pod *corev1.Pod) []resources.ResourceRef {
	if client == nil || pod == nil || pod.Namespace == "" || pod.Name == "" || pod.UID == "" {
		return []resources.ResourceRef{}
	}
	refs := make([]resources.ResourceRef, 0, 100)
	seen := map[string]struct{}{}
	add := func(apiGroup, kind string, object metav1.Object) {
		if object.GetNamespace() != pod.Namespace || object.GetName() == "" || object.GetUID() == "" || len(refs) >= 100 {
			return
		}
		// The same Event may be served through core/v1 and events.k8s.io/v1;
		// Kubernetes UID is the cross-version identity.
		key := string(object.GetUID())
		if _, duplicate := seen[key]; duplicate {
			return
		}
		seen[key] = struct{}{}
		refs = append(refs, resources.ResourceRef{APIGroup: apiGroup, Kind: kind, Namespace: object.GetNamespace(), Name: object.GetName(), UID: string(object.GetUID())})
	}
	selector := "involvedObject.uid=" + string(pod.UID)
	if backend.canListNamespace(ctx, binding, pod.Namespace, "", "events") {
		if list, err := client.CoreV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{FieldSelector: selector, Limit: 100}); err == nil {
			for index := range list.Items {
				event := &list.Items[index]
				if event.InvolvedObject.UID == pod.UID && event.InvolvedObject.Kind == "Pod" && event.InvolvedObject.Namespace == pod.Namespace && event.InvolvedObject.Name == pod.Name {
					add("", "Event", event)
				}
			}
		}
	}
	if len(refs) < 100 && backend.canListNamespace(ctx, binding, pod.Namespace, "events.k8s.io", "events") {
		selector = "regarding.uid=" + string(pod.UID)
		if list, err := client.EventsV1().Events(pod.Namespace).List(ctx, metav1.ListOptions{FieldSelector: selector, Limit: 100}); err == nil {
			for index := range list.Items {
				event := &list.Items[index]
				if event.Regarding.UID == pod.UID && event.Regarding.Kind == "Pod" && event.Regarding.Namespace == pod.Namespace && event.Regarding.Name == pod.Name {
					add(eventsv1.SchemeGroupVersion.Group, "Event", event)
				}
			}
		}
	}
	sortResourceRefs(refs)
	return refs
}

func (backend *ResourceBackend) relatedWorkload(ctx context.Context, binding namespaces.SelectionBinding, value any) ([]resources.ResourceRef, []batchv1.Job, bool) {
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil || clients.kubernetes == nil {
		if cancel != nil {
			cancel()
		}
		return []resources.ResourceRef{}, nil, false
	}
	defer cancel()
	refs := []resources.ResourceRef{}
	jobs := []batchv1.Job(nil)
	historyComplete := true
	var namespace string
	var podOwnerUIDs map[types.UID]struct{}
	var podSelector string
	add := func(apiGroup, kind string, object metav1.Object) {
		if len(refs) == maximumWorkloadRelated || object.GetName() == "" || object.GetUID() == "" {
			return
		}
		refs = append(refs, resources.ResourceRef{APIGroup: apiGroup, Kind: kind, Namespace: object.GetNamespace(), Name: object.GetName(), UID: string(object.GetUID())})
	}
	switch object := value.(type) {
	case *appsv1.Deployment:
		namespace, podSelector = object.Namespace, labelSelector(object.Spec.Selector)
		podOwnerUIDs = map[types.UID]struct{}{}
		if backend.canListNamespace(requestContext, binding, namespace, "apps", "replicasets") {
			for _, replicaSet := range listReplicaSets(requestContext, clients.kubernetes, namespace, podSelector) {
				if ownedBy(replicaSet.OwnerReferences, object.UID) {
					add("apps", "ReplicaSet", &replicaSet)
					podOwnerUIDs[replicaSet.UID] = struct{}{}
				}
			}
		}
	case *appsv1.StatefulSet:
		namespace, podSelector = object.Namespace, labelSelector(object.Spec.Selector)
		podOwnerUIDs = map[types.UID]struct{}{object.UID: {}}
	case *appsv1.DaemonSet:
		namespace, podSelector = object.Namespace, labelSelector(object.Spec.Selector)
		podOwnerUIDs = map[types.UID]struct{}{object.UID: {}}
	case *appsv1.ReplicaSet:
		namespace, podSelector = object.Namespace, labelSelector(object.Spec.Selector)
		podOwnerUIDs = map[types.UID]struct{}{object.UID: {}}
	case *batchv1.Job:
		namespace, podSelector = object.Namespace, labelSelector(object.Spec.Selector)
		podOwnerUIDs = map[types.UID]struct{}{object.UID: {}}
	case *batchv1.CronJob:
		namespace = object.Namespace
		jobs, historyComplete = backend.cronJobHistory(requestContext, binding, clients.kubernetes, namespace)
		historyComplete = historyComplete && object.UID != ""
		podOwnerUIDs = map[types.UID]struct{}{}
		for index := range jobs {
			if ownedBy(jobs[index].OwnerReferences, object.UID) {
				add("batch", "Job", &jobs[index])
				podOwnerUIDs[jobs[index].UID] = struct{}{}
			}
		}
	default:
		return refs, jobs, false
	}
	if len(refs) < maximumWorkloadRelated && len(podOwnerUIDs) > 0 && backend.canListNamespace(requestContext, binding, namespace, "", "pods") {
		for _, pod := range listPods(requestContext, clients.kubernetes, namespace, podSelector) {
			if ownedByAny(pod.OwnerReferences, podOwnerUIDs) {
				add("", "Pod", &pod)
			}
		}
	}
	sortResourceRefs(refs)
	if len(refs) > maximumWorkloadRelated {
		refs = refs[:maximumWorkloadRelated]
	}
	return refs, jobs, historyComplete
}

func listReplicaSets(ctx context.Context, client kubeclient.Interface, namespace, selector string) []appsv1.ReplicaSet {
	result := []appsv1.ReplicaSet{}
	continueToken := ""
	for len(result) < maximumRelatedScanItems {
		list, err := client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector, Limit: relatedListPageSize, Continue: continueToken})
		if err != nil {
			return nil
		}
		result = append(result, list.Items...)
		continueToken = list.Continue
		if continueToken == "" {
			break
		}
	}
	if len(result) > maximumRelatedScanItems {
		result = result[:maximumRelatedScanItems]
	}
	return result
}

func listPods(ctx context.Context, client kubeclient.Interface, namespace, selector string) []corev1.Pod {
	result := []corev1.Pod{}
	continueToken := ""
	for len(result) < maximumRelatedScanItems {
		list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector, Limit: relatedListPageSize, Continue: continueToken})
		if err != nil {
			return nil
		}
		result = append(result, list.Items...)
		continueToken = list.Continue
		if continueToken == "" {
			break
		}
	}
	if len(result) > maximumRelatedScanItems {
		result = result[:maximumRelatedScanItems]
	}
	return result
}

func labelSelector(selector *metav1.LabelSelector) string {
	if selector == nil {
		return ""
	}
	parsed, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return ""
	}
	return parsed.String()
}

func ownedBy(references []metav1.OwnerReference, uid types.UID) bool {
	if uid == "" {
		return false
	}
	for _, reference := range references {
		if reference.UID == uid {
			return true
		}
	}
	return false
}

func ownedByAny(references []metav1.OwnerReference, uids map[types.UID]struct{}) bool {
	for _, reference := range references {
		if _, ok := uids[reference.UID]; ok && reference.UID != "" {
			return true
		}
	}
	return false
}

func sortResourceRefs(refs []resources.ResourceRef) {
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		if refs[i].Namespace != refs[j].Namespace {
			return refs[i].Namespace < refs[j].Namespace
		}
		if refs[i].Name != refs[j].Name {
			return refs[i].Name < refs[j].Name
		}
		return refs[i].UID < refs[j].UID
	})
}
