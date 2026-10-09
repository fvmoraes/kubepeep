package resources

import (
	"k8s.io/apimachinery/pkg/fields"
	"time"
)

const (
	CollectionHelmReleases          Collection = "helm-releases"
	CollectionHelmConfigMapReleases Collection = "helm-configmap-releases"
	MaximumHelmDocumentBytes                   = 2 << 20
	MaximumHelmMetadataItems                   = 10000
)

func init() {
	for collection, resource := range map[Collection]string{CollectionHelmReleases: "secrets", CollectionHelmConfigMapReleases: "configmaps"} {
		rulesByCollection[collection] = collectionRules{sorts: []string{"identity", "name"}, defaultSort: "identity", defaultOrder: OrderAscending}
		collectionGVR[collection] = Origin{Version: "v1", Resource: resource}
		columnCollectionIDs = append(columnCollectionIDs, string(collection))
	}
}

type HelmReleaseDTO struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	Driver          string            `json:"driver"`
	Status          string            `json:"status"`
	Revision        int               `json:"revision"`
	StorageName     string            `json:"storageName"`
	UID             string            `json:"uid"`
	ResourceVersion string            `json:"resourceVersion"`
	AgeSeconds      int64             `json:"ageSeconds"`
	Chart           string            `json:"chart,omitempty"`
	AppVersion      string            `json:"appVersion,omitempty"`
	UpdatedAt       string            `json:"updatedAt,omitempty"`
	Related         []ResourceRef     `json:"related"`
	History         []HelmRevisionDTO `json:"history,omitempty"`
}

func (HelmReleaseDTO) resourceListItem()   {}
func (HelmReleaseDTO) resourceDetailItem() {}

type HelmRevisionDTO struct {
	Revision  int    `json:"revision"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

type HelmDocumentDTO struct {
	Document        string `json:"document"`
	Format          string `json:"format"`
	Revision        int    `json:"revision"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resourceVersion"`
}

type HelmMutationRequest struct {
	ExpectedGeneration      string `json:"expectedGeneration"`
	ExpectedRevision        int    `json:"expectedRevision"`
	ExpectedUID             string `json:"expectedUid"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
	Confirmed               bool   `json:"confirmed"`
	Action                  string `json:"action"`
	Values                  string `json:"values,omitempty"`
	Revision                int    `json:"revision,omitempty"`
}

type HelmMutationResult struct {
	Accepted bool   `json:"accepted"`
	Revision int    `json:"revision"`
	Status   string `json:"status"`
}

func HelmCollection(driver string) (Collection, bool) {
	switch driver {
	case "secrets":
		return CollectionHelmReleases, true
	case "configmaps":
		return CollectionHelmConfigMapReleases, true
	}
	return "", false
}

func HelmAge(created time.Time, now time.Time) int64 {
	if created.IsZero() {
		return 0
	}
	return max(0, int64(now.Sub(created).Seconds()))
}

func HelmFieldSelector(value string) (func(string, string) bool, error) {
	selector, err := fields.ParseSelector(value)
	if err != nil {
		return nil, validationError("invalid Helm field selector")
	}
	return func(name, namespace string) bool {
		return selector.Matches(fields.Set{"metadata.name": name, "metadata.namespace": namespace})
	}, nil
}
