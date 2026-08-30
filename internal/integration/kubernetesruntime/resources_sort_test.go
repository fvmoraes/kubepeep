package kubernetesruntime

import (
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

func TestNaturalTextCompareUsesNumericRunsWithoutIntegerParsing(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
		want        int
	}{
		{name: "numeric suffix", left: "pod-2", right: "pod-10", want: -1},
		{name: "reverse numeric suffix", left: "pod-10", right: "pod-2", want: 1},
		{name: "equivalent leading zeroes", left: "pod-02", right: "pod-2", want: 0},
		{name: "multiple numeric runs", left: "job-2-revision-9", right: "job-2-revision-10", want: -1},
		{name: "unbounded numeric run", left: "pod-99999999999999999999", right: "pod-100000000000000000000", want: -1},
		{name: "ordinary text", left: "alpha", right: "beta", want: -1},
		{name: "equal", left: "pod-2", right: "pod-2", want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := naturalTextCompare(test.left, test.right); got != test.want {
				t.Fatalf("naturalTextCompare(%q, %q)=%d, want %d", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestPageLocalPodNameSortIsNaturalAndKeepsCanonicalTiesAscending(t *testing.T) {
	newPods := func() []resources.PodDTO {
		return []resources.PodDTO{
			{Namespace: "team-b", Name: "pod-2"},
			{Namespace: "team-a", Name: "pod-10"},
			{Namespace: "team-a", Name: "pod-2"},
		}
	}

	ascending := filterSortPods(newPods(), resources.ListOptions{Sort: "name", Order: resources.OrderAscending})
	assertPodOrder(t, ascending, []string{"team-a/pod-2", "team-b/pod-2", "team-a/pod-10"})

	descending := filterSortPods(newPods(), resources.ListOptions{Sort: "name", Order: resources.OrderDescending})
	assertPodOrder(t, descending, []string{"team-a/pod-10", "team-a/pod-2", "team-b/pod-2"})
}

func TestPageLocalTextSortKeysAreNaturalAcrossResourceCollections(t *testing.T) {
	t.Run("workload status", func(t *testing.T) {
		items := []resources.WorkloadDTO{
			{Kind: "Deployment", Namespace: "default", Name: "ten", Status: resources.WorkloadStatus("Phase-10")},
			{Kind: "Deployment", Namespace: "default", Name: "two", Status: resources.WorkloadStatus("Phase-2")},
		}
		got := filterSortWorkloads(items, resources.ListOptions{Sort: "status", Order: resources.OrderAscending})
		if got[0].Name != "two" {
			t.Fatalf("workload status order=%#v", got)
		}
	})

	t.Run("event identity", func(t *testing.T) {
		items := []resources.EventDTO{
			{Namespace: "default", ObjectKind: "Pod", ObjectName: "pod-10", Reason: "Started"},
			{Namespace: "default", ObjectKind: "Pod", ObjectName: "pod-2", Reason: "Started"},
		}
		got := filterSortEvents(items, resources.ListOptions{Sort: "identity", Order: resources.OrderAscending})
		if got[0].ObjectName != "pod-2" {
			t.Fatalf("event identity order=%#v", got)
		}
	})

	t.Run("service type", func(t *testing.T) {
		items := []resources.ServiceDTO{
			{Namespace: "default", Name: "ten", Type: "External-10"},
			{Namespace: "default", Name: "two", Type: "External-2"},
		}
		got := filterSortServices(items, resources.ListOptions{Sort: "type", Order: resources.OrderAscending})
		if got[0].Name != "two" {
			t.Fatalf("service type order=%#v", got)
		}
	})

	t.Run("ingress name", func(t *testing.T) {
		items := []resources.IngressDTO{{Namespace: "default", Name: "route-10"}, {Namespace: "default", Name: "route-2"}}
		got := filterSortIngresses(items, resources.ListOptions{Sort: "name", Order: resources.OrderAscending})
		if got[0].Name != "route-2" {
			t.Fatalf("ingress name order=%#v", got)
		}
	})

	t.Run("endpoint slice address type", func(t *testing.T) {
		items := []resources.EndpointSliceDTO{
			{Namespace: "default", Name: "ten", AddressType: "IPv10"},
			{Namespace: "default", Name: "two", AddressType: "IPv2"},
		}
		got := filterSortEndpointSlices(items, resources.ListOptions{Sort: "addressType", Order: resources.OrderAscending})
		if got[0].Name != "two" {
			t.Fatalf("endpoint slice address type order=%#v", got)
		}
	})

	t.Run("config map name", func(t *testing.T) {
		items := []resources.ConfigMapListDTO{{Namespace: "default", Name: "config-10", UID: "ten"}, {Namespace: "default", Name: "config-2", UID: "two"}}
		got := filterSortConfigMaps(items, resources.ListOptions{Sort: "name", Order: resources.OrderAscending})
		if got[0].Name != "config-2" {
			t.Fatalf("config map name order=%#v", got)
		}
	})

	t.Run("secret name", func(t *testing.T) {
		items := []resources.SecretMetadataDTO{
			{Metadata: resources.SecretMetadataFieldsDTO{Namespace: "default", Name: "secret-10", UID: "ten"}},
			{Metadata: resources.SecretMetadataFieldsDTO{Namespace: "default", Name: "secret-2", UID: "two"}},
		}
		got := filterSortSecrets(items, resources.ListOptions{Sort: "name", Order: resources.OrderAscending})
		if got[0].Metadata.Name != "secret-2" {
			t.Fatalf("secret name order=%#v", got)
		}
	})
}

func assertPodOrder(t *testing.T, pods []resources.PodDTO, want []string) {
	t.Helper()
	if len(pods) != len(want) {
		t.Fatalf("pod count=%d, want %d", len(pods), len(want))
	}
	for index, pod := range pods {
		if got := pod.Namespace + "/" + pod.Name; got != want[index] {
			t.Fatalf("pod[%d]=%q, want %q; all=%#v", index, got, want[index], pods)
		}
	}
}
