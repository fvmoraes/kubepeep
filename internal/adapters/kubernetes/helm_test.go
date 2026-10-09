package kubernetes

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientfake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	clienttesting "k8s.io/client-go/testing"
)

func helmStoredRelease(revision int) *release.Release {
	return &release.Release{Name: "demo", Namespace: "ns", Version: revision, Info: &release.Info{Status: release.StatusDeployed}, Chart: &chart.Chart{Metadata: &chart.Metadata{Name: "demo", Version: "1.0.0", APIVersion: "v2"}, Values: map[string]interface{}{"replicas": 1}}, Config: map[string]interface{}{"replicas": 1}}
}
func testHelmStorage(t *testing.T, resource string, count int) (*helmStorage, *clientfake.Clientset) {
	t.Helper()
	core := clientfake.NewSimpleClientset()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: resource}
	kind := "Secret"
	if resource == "configmaps" {
		kind = "ConfigMap"
	}
	for revision := 1; revision <= count; revision++ {
		value := helmStoredRelease(revision)
		if revision < count {
			value.Info.Status = release.StatusSuperseded
		}
		data, err := encodeBoundedHelmRelease(value)
		if err != nil {
			t.Fatal(err)
		}
		object := metav1.ObjectMeta{Name: helmKey("demo", revision), Namespace: "ns", UID: types.UID(fmt.Sprintf("uid-%d", revision)), ResourceVersion: "1", Labels: map[string]string{"owner": "helm", "name": "demo", "version": fmt.Sprint(revision), "status": value.Info.Status.String()}}
		if resource == "secrets" {
			err = core.Tracker().Add(&corev1.Secret{ObjectMeta: object, Data: map[string][]byte{"release": data}})
		} else {
			err = core.Tracker().Add(&corev1.ConfigMap{ObjectMeta: object, Data: map[string]string{"release": string(data)}})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	metadata := metadatafake.NewSimpleMetadataClient(metadatafake.NewTestScheme())
	metadata.PrependReactor("list", resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
		raw, err := core.Tracker().List(gvr, schema.GroupVersionKind{Version: "v1", Kind: kind}, action.GetNamespace())
		if err != nil {
			return true, nil, err
		}
		result := &metav1.List{}
		selector := action.(clienttesting.ListAction).GetListRestrictions().Labels
		if list, ok := raw.(*corev1.SecretList); ok {
			for _, item := range list.Items {
				if selector.Matches(labels.Set(item.Labels)) {
					result.Items = append(result.Items, runtime.RawExtension{Object: &metav1.PartialObjectMetadata{ObjectMeta: item.ObjectMeta}})
				}
			}
		} else {
			for _, item := range raw.(*corev1.ConfigMapList).Items {
				if selector.Matches(labels.Set(item.Labels)) {
					result.Items = append(result.Items, runtime.RawExtension{Object: &metav1.PartialObjectMetadata{ObjectMeta: item.ObjectMeta}})
				}
			}
		}
		return true, result, nil
	})
	core.PrependReactor("create", resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
		object := action.(clienttesting.CreateAction).GetObject()
		switch value := object.(type) {
		case *corev1.Secret:
			value.UID = types.UID("new-" + value.Name)
			value.ResourceVersion = "1"
		case *corev1.ConfigMap:
			value.UID = types.UID("new-" + value.Name)
			value.ResourceVersion = "1"
		}
		return false, nil, nil
	})
	return &helmStorage{ctx: t.Context(), core: core.CoreV1(), metadata: metadata, namespace: "ns", resource: resource, seen: map[string]helmRecordIdentity{}}, core
}

func TestHelmStorageBoundsCompressedReleaseBeforeSDK(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := io.Copy(writer, strings.NewReader(strings.Repeat("x", MaximumHelmReleaseBytes+1))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		data []byte
		want error
	}{{"gzip expansion", []byte(base64.StdEncoding.EncodeToString(compressed.Bytes())), ErrHelmReleaseLimit}, {"invalid base64", []byte("secret-value!"), ErrHelmInvalidStorage}, {"nil info", []byte(base64.StdEncoding.EncodeToString([]byte(`{"name":"demo","namespace":"ns","version":1}`))), ErrHelmInvalidStorage}} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := decodeBoundedHelmRelease(test.data); !errors.Is(err, test.want) {
				t.Fatalf("unexpected safe decode error: %v", err)
			}
		})
	}
}
func TestHelmStorageQueriesMetadataAndNeverListsFullSecrets(t *testing.T) {
	for _, resource := range []string{"secrets", "configmaps"} {
		t.Run(resource, func(t *testing.T) {
			store, core := testHelmStorage(t, resource, 3)
			values, err := store.Query(map[string]string{"name": "demo", "owner": "helm"})
			if err != nil || len(values) != 3 {
				t.Fatalf("query: %d %v", len(values), err)
			}
			for _, call := range core.Actions() {
				if call.GetVerb() != "get" {
					t.Fatalf("non-GET payload access: %s", call.GetVerb())
				}
			}
		})
	}
}
func TestHelmRevisionGuardRejectsConcurrentRevisionAndMetadataChanges(t *testing.T) {
	for _, change := range []string{"revision", "uid", "resourceVersion"} {
		t.Run(change, func(t *testing.T) {
			store, core := testHelmStorage(t, "secrets", 1)
			store.guard = &HelmRevisionGuard{Name: "demo", Revision: 1, UID: "uid-1", ResourceVersion: "1"}
			if change == "revision" {
				value := helmStoredRelease(2)
				data, _ := encodeBoundedHelmRelease(value)
				if err := core.Tracker().Add(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: helmKey("demo", 2), Namespace: "ns", UID: "uid-2", ResourceVersion: "1", Labels: map[string]string{"owner": "helm", "name": "demo", "version": "2", "status": "deployed"}}, Data: map[string][]byte{"release": data}}); err != nil {
					t.Fatal(err)
				}
			} else {
				raw, _ := core.Tracker().Get(schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, "ns", helmKey("demo", 1))
				record := raw.(*corev1.Secret)
				if change == "uid" {
					record.UID = "different"
				} else {
					record.ResourceVersion = "2"
				}
				if err := core.Tracker().Update(schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, record, "ns"); err != nil {
					t.Fatal(err)
				}
			}
			err := store.Create(helmKey("demo", 2), helmStoredRelease(2))
			if !apierrors.IsConflict(err) {
				t.Fatalf("concurrent %s not rejected: %v", change, err)
			}
			for _, call := range core.Actions() {
				if call.GetVerb() == "create" || call.GetVerb() == "update" || call.GetVerb() == "delete" {
					t.Fatal("stale revision mutated storage")
				}
			}
		})
	}
}
func TestHelmSDKUpgradeAndRollbackKeepHistoryAndUseStoragePreconditions(t *testing.T) {
	for _, resource := range []string{"secrets", "configmaps"} {
		for _, operation := range []string{"values", "rollback"} {
			t.Run(resource+"/"+operation, func(t *testing.T) {
				store, core := testHelmStorage(t, resource, 25)
				configuration := &action.Configuration{Releases: storage.Init(store), KubeClient: &kubefake.PrintingKubeClient{Out: io.Discard, LogOutput: io.Discard}, Capabilities: chartutil.DefaultCapabilities, Log: func(string, ...interface{}) {}}
				client := &HelmClient{configuration: configuration, namespace: "ns", storage: store}
				if err := client.RequireRevision(HelmRevisionGuard{Name: "demo", Revision: 25, UID: "uid-25", ResourceVersion: "1"}); err != nil {
					t.Fatal(err)
				}
				var value *release.Release
				var err error
				if operation == "values" {
					value, err = client.Upgrade(t.Context(), "demo", helmStoredRelease(25).Chart, map[string]interface{}{"replicas": 3})
				} else {
					value, err = client.Rollback("demo", 3)
				}
				if err != nil || value == nil || value.Version != 26 {
					t.Fatalf("SDK %s failed: %#v %v", operation, value, err)
				}
				updates := 0
				for _, call := range core.Actions() {
					if call.GetVerb() == "delete" {
						t.Fatal("history pruned")
					}
					if call.GetVerb() == "update" {
						updates++
						object := call.(clienttesting.UpdateAction).GetObject()
						var uid types.UID
						var rv string
						switch x := object.(type) {
						case *corev1.Secret:
							uid = x.UID
							rv = x.ResourceVersion
						case *corev1.ConfigMap:
							uid = x.UID
							rv = x.ResourceVersion
						}
						if uid == "" || rv == "" {
							t.Fatal("storage update omitted UID/resourceVersion")
						}
					}
				}
				if updates == 0 {
					t.Fatal("SDK did not update release status")
				}
				if configuration.Releases.MaxHistory != 0 {
					t.Fatal("history retention changed")
				}
			})
		}
	}
}

type helmRoundTripFunc func(*http.Request) (*http.Response, error)

func (f helmRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestHelmTransportCancelsRequestsWhenGenerationEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})
	done := make(chan error, 1)
	transport := helmTransport{ctx: ctx, base: helmRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	go func() {
		request, _ := http.NewRequest(http.MethodGet, "https://cluster.invalid", nil)
		_, err := transport.RoundTrip(request)
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("generation did not cancel transport: %v", err)
	}
}
