package kubernetesruntime

import (
	"testing"

	"github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

func TestActivationFenceRejectsOlderGenerationAndLeasePublication(t *testing.T) {
	runtime := &Runtime{binding: namespaces.SelectionBinding{
		ClusterProfileID: 1, Context: "old", Cluster: "cluster-old", Generation: "gen_new",
	}}
	stale := namespaces.SelectionBinding{
		ClusterProfileID: 1, Context: "old", Cluster: "cluster-old", Generation: "gen_old",
	}
	if runtime.beginActivation(&candidate{}, stale) {
		t.Fatal("older generation crossed the activation fence")
	}

	current := namespaces.SelectionBinding{
		ClusterProfileID: 2, Context: "new", Cluster: "cluster-new", Generation: "gen_new",
	}
	if !runtime.beginActivation(&candidate{}, current) {
		t.Fatal("current generation was rejected")
	}
	runtime.OnGeneration("gen_next")
	if runtime.publishLease(current, &kubernetes.Lease{}) {
		t.Fatal("lease from the previous generation was published")
	}
	if runtime.matchesBinding(current) {
		t.Fatal("previous binding still appears current")
	}
}

func TestSameBindingIncludesScopeAndCluster(t *testing.T) {
	base := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Cluster: "cluster-a", ActiveScopeID: 4, Generation: "gen"}
	if !sameBinding(base, base) {
		t.Fatal("identical bindings differ")
	}
	changedScope := base
	changedScope.ActiveScopeID++
	if sameBinding(base, changedScope) {
		t.Fatal("scope change was ignored")
	}
	changedCluster := base
	changedCluster.Cluster = "cluster-b"
	if sameBinding(base, changedCluster) {
		t.Fatal("cluster change was ignored")
	}
}
