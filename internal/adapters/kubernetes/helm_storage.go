package kubernetes

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/metadata"
)

const (
	MaximumHelmEncodedBytes   = 2 << 20
	MaximumHelmReleaseBytes   = 16 << 20
	maximumHelmQueryBytes     = 64 << 20
	maximumHelmStorageRecords = 10000
)

var (
	ErrHelmReleaseLimit   = errors.New("helm release exceeds the bounded storage read limit")
	ErrHelmInvalidStorage = errors.New("helm release storage is invalid")
	ErrHelmReadOnly       = errors.New("helm storage mutation is not authorized for this operation")
)

type HelmRevisionGuard struct {
	Name                 string
	Revision             int
	UID, ResourceVersion string
}
type helmRecordIdentity struct {
	UID             types.UID
	ResourceVersion string
}

// helmStorage implements the SDK storage contract without its unbounded gzip
// decoder and unpaginated Secret LIST. Values exist only for this request.
// History deletion is deliberately unavailable; upgrades never prune records.
type helmStorage struct {
	ctx                 context.Context
	core                coreclient.CoreV1Interface
	metadata            metadata.Interface
	namespace, resource string
	mu                  sync.Mutex
	seen                map[string]helmRecordIdentity
	recordMetadata      map[string]metav1.ObjectMeta
	guard               *HelmRevisionGuard
	created             bool
}

func (s *helmStorage) Name() string {
	if s.resource == "secrets" {
		return driver.SecretsDriverName
	}
	return driver.ConfigMapsDriverName
}
func helmKey(name string, revision int) string {
	return fmt.Sprintf("sh.helm.release.v1.%s.v%d", name, revision)
}
func (s *helmStorage) conflict() error {
	return apierrors.NewConflict(schema.GroupResource{Resource: s.resource}, "", errors.New("helm release changed during the operation"))
}

func decodeBoundedHelmRelease(encoded []byte) (*release.Release, int, error) {
	if len(encoded) > MaximumHelmEncodedBytes {
		return nil, 0, ErrHelmReleaseLimit
	}
	compressed, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, 0, ErrHelmInvalidStorage
	}
	raw := compressed
	if len(compressed) >= 3 && bytes.Equal(compressed[:3], []byte{0x1f, 0x8b, 0x08}) {
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, 0, ErrHelmInvalidStorage
		}
		raw, err = io.ReadAll(io.LimitReader(reader, MaximumHelmReleaseBytes+1))
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			return nil, 0, ErrHelmInvalidStorage
		}
	}
	if len(raw) > MaximumHelmReleaseBytes {
		return nil, 0, ErrHelmReleaseLimit
	}
	var value release.Release
	if json.Unmarshal(raw, &value) != nil || value.Info == nil || value.Name == "" || value.Namespace == "" || value.Version < 1 {
		return nil, 0, ErrHelmInvalidStorage
	}
	return &value, len(raw), nil
}
func encodeBoundedHelmRelease(value *release.Release) ([]byte, error) {
	if value == nil || value.Info == nil {
		return nil, ErrHelmInvalidStorage
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, ErrHelmInvalidStorage
	}
	if len(raw) > MaximumHelmReleaseBytes {
		return nil, ErrHelmReleaseLimit
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(raw); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if base64.StdEncoding.EncodedLen(compressed.Len()) > MaximumHelmEncodedBytes {
		return nil, ErrHelmReleaseLimit
	}
	return []byte(base64.StdEncoding.EncodeToString(compressed.Bytes())), nil
}
func (s *helmStorage) read(key string) (*release.Release, int, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, 0, err
	}
	var data []byte
	var object metav1.Object
	var recordMetadata metav1.ObjectMeta
	if s.resource == "secrets" {
		value, err := s.core.Secrets(s.namespace).Get(s.ctx, key, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, 0, driver.ErrReleaseNotFound
			}
			return nil, 0, err
		}
		data, object = value.Data["release"], value
		recordMetadata = value.ObjectMeta
	} else {
		value, err := s.core.ConfigMaps(s.namespace).Get(s.ctx, key, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, 0, driver.ErrReleaseNotFound
			}
			return nil, 0, err
		}
		data, object = []byte(value.Data["release"]), value
		recordMetadata = value.ObjectMeta
	}
	value, size, err := decodeBoundedHelmRelease(data)
	if err != nil {
		return nil, 0, err
	}
	if value.Namespace != s.namespace || key != helmKey(value.Name, value.Version) || object.GetLabels()["owner"] != "helm" || object.GetLabels()["name"] != value.Name || object.GetLabels()["version"] != strconv.Itoa(value.Version) || object.GetLabels()["status"] != value.Info.Status.String() {
		return nil, 0, ErrHelmInvalidStorage
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.guard != nil && key == helmKey(s.guard.Name, s.guard.Revision) && !s.created && (string(object.GetUID()) != s.guard.UID || object.GetResourceVersion() != s.guard.ResourceVersion) {
		return nil, 0, s.conflict()
	}
	identity := helmRecordIdentity{UID: object.GetUID(), ResourceVersion: object.GetResourceVersion()}
	if prior, ok := s.seen[key]; ok && prior != identity {
		return nil, 0, s.conflict()
	}
	s.seen[key] = identity
	if s.recordMetadata == nil {
		s.recordMetadata = map[string]metav1.ObjectMeta{}
	}
	s.recordMetadata[key] = *recordMetadata.DeepCopy()
	value.Labels = map[string]string{}
	for key, valueLabel := range object.GetLabels() {
		if key != "owner" && key != "name" && key != "status" && key != "version" && key != "createdAt" && key != "modifiedAt" {
			value.Labels[key] = valueLabel
		}
	}
	return value, size, nil
}
func (s *helmStorage) Get(key string) (*release.Release, error) {
	value, _, err := s.read(key)
	return value, err
}
func (s *helmStorage) metadataRecords(selector labels.Set) ([]metav1.PartialObjectMetadata, error) {
	copy := labels.Set{"owner": "helm"}
	for key, value := range selector {
		copy[key] = value
	}
	selector = copy
	selector["owner"] = "helm"
	options := metav1.ListOptions{LabelSelector: selector.AsSelector().String(), Limit: 200}
	records := []metav1.PartialObjectMetadata{}
	seenTokens := map[string]bool{}
	for {
		if err := s.ctx.Err(); err != nil {
			return nil, err
		}
		list, err := s.metadata.Resource(schema.GroupVersionResource{Version: "v1", Resource: s.resource}).Namespace(s.namespace).List(s.ctx, options)
		if err != nil {
			return nil, err
		}
		if len(records)+len(list.Items) > maximumHelmStorageRecords {
			return nil, ErrHelmReleaseLimit
		}
		records = append(records, list.Items...)
		if list.Continue == "" {
			return records, nil
		}
		if list.Continue == options.Continue || seenTokens[list.Continue] || len(seenTokens) >= maximumHelmStorageRecords {
			return nil, ErrHelmInvalidStorage
		}
		seenTokens[list.Continue] = true
		options.Continue = list.Continue
	}
}
func (s *helmStorage) Query(selector map[string]string) ([]*release.Release, error) {
	metadata, err := s.metadataRecords(labels.Set(selector))
	if err != nil {
		return nil, err
	}
	if len(metadata) == 0 {
		return nil, driver.ErrReleaseNotFound
	}
	s.mu.Lock()
	guard := s.guard
	created := s.created
	s.mu.Unlock()
	if guard != nil && selector["name"] != guard.Name {
		return nil, ErrHelmReadOnly
	}
	if guard != nil && selector["status"] == "" {
		latest := 0
		for _, record := range metadata {
			revision, parseErr := strconv.Atoi(record.Labels["version"])
			if parseErr != nil {
				return nil, ErrHelmInvalidStorage
			}
			latest = max(latest, revision)
		}
		expected := guard.Revision
		if created {
			expected++
		}
		if latest != expected {
			return nil, s.conflict()
		}
	}
	result := make([]*release.Release, 0, len(metadata))
	total := 0
	for _, record := range metadata {
		value, size, err := s.read(record.Name)
		if err != nil {
			return nil, err
		}
		total += size
		if total > maximumHelmQueryBytes {
			return nil, ErrHelmReleaseLimit
		}
		result = append(result, value)
	}
	return result, nil
}
func (s *helmStorage) List(filter func(*release.Release) bool) ([]*release.Release, error) {
	values, err := s.Query(map[string]string{"owner": "helm"})
	if err != nil {
		return nil, err
	}
	result := []*release.Release{}
	for _, value := range values {
		if filter(value) {
			result = append(result, value)
		}
	}
	return result, nil
}
func (s *helmStorage) Create(key string, value *release.Release) error {
	s.mu.Lock()
	guard := s.guard
	created := s.created
	s.mu.Unlock()
	if guard == nil || created || value == nil || value.Name != guard.Name || value.Namespace != s.namespace || value.Version != guard.Revision+1 || key != helmKey(value.Name, value.Version) {
		return ErrHelmReadOnly
	}
	// Recheck the current metadata and revision immediately before acquiring
	// Helm's standard N+1 lock. A concurrent upgrade then loses the CREATE race.
	records, err := s.metadataRecords(labels.Set{"name": guard.Name})
	if err != nil {
		return err
	}
	latest := 0
	for _, record := range records {
		revision, parseErr := strconv.Atoi(record.Labels["version"])
		if parseErr != nil {
			return ErrHelmInvalidStorage
		}
		latest = max(latest, revision)
	}
	if latest != guard.Revision {
		return s.conflict()
	}
	if _, err := s.Get(helmKey(guard.Name, guard.Revision)); err != nil {
		return err
	}
	data, err := encodeBoundedHelmRelease(value)
	if err != nil {
		return err
	}
	object := s.objectMeta(key, value)
	object.Labels["createdAt"] = strconv.FormatInt(time.Now().Unix(), 10)
	var identity helmRecordIdentity
	var savedMetadata metav1.ObjectMeta
	if s.resource == "secrets" {
		saved, err := s.core.Secrets(s.namespace).Create(s.ctx, &corev1.Secret{ObjectMeta: object, Type: "helm.sh/release.v1", Data: map[string][]byte{"release": data}}, metav1.CreateOptions{})
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				return s.conflict()
			}
			return err
		}
		identity = helmRecordIdentity{UID: saved.UID, ResourceVersion: saved.ResourceVersion}
		savedMetadata = saved.ObjectMeta
	} else {
		saved, err := s.core.ConfigMaps(s.namespace).Create(s.ctx, &corev1.ConfigMap{ObjectMeta: object, Data: map[string]string{"release": string(data)}}, metav1.CreateOptions{})
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				return s.conflict()
			}
			return err
		}
		identity = helmRecordIdentity{UID: saved.UID, ResourceVersion: saved.ResourceVersion}
		savedMetadata = saved.ObjectMeta
	}
	s.mu.Lock()
	s.created = true
	s.seen[key] = identity
	s.recordMetadata[key] = *savedMetadata.DeepCopy()
	s.mu.Unlock()
	return nil
}
func (s *helmStorage) objectMeta(key string, value *release.Release) metav1.ObjectMeta {
	labels := map[string]string{}
	for k, v := range value.Labels {
		labels[k] = v
	}
	labels["owner"] = "helm"
	labels["name"] = value.Name
	labels["version"] = strconv.Itoa(value.Version)
	labels["status"] = value.Info.Status.String()
	return metav1.ObjectMeta{Name: key, Namespace: s.namespace, Labels: labels}
}
func (s *helmStorage) Update(key string, value *release.Release) error {
	s.mu.Lock()
	guard := s.guard
	identity, known := s.seen[key]
	priorMetadata := s.recordMetadata[key]
	created := s.created
	s.mu.Unlock()
	if guard == nil || !created || !known || value == nil || value.Name != guard.Name || key != helmKey(value.Name, value.Version) || value.Namespace != s.namespace || value.Version > guard.Revision+1 {
		return ErrHelmReadOnly
	}
	if identity.UID == "" || identity.ResourceVersion == "" {
		return s.conflict()
	}
	data, err := encodeBoundedHelmRelease(value)
	if err != nil {
		return err
	}
	object := *priorMetadata.DeepCopy()
	if object.Labels == nil {
		object.Labels = map[string]string{}
	}
	for name, label := range s.objectMeta(key, value).Labels {
		object.Labels[name] = label
	}
	object.UID = identity.UID
	object.ResourceVersion = identity.ResourceVersion
	object.Labels["modifiedAt"] = strconv.FormatInt(time.Now().Unix(), 10)
	var savedMetadata metav1.ObjectMeta
	if s.resource == "secrets" {
		saved, err := s.core.Secrets(s.namespace).Update(s.ctx, &corev1.Secret{ObjectMeta: object, Type: "helm.sh/release.v1", Data: map[string][]byte{"release": data}}, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		identity = helmRecordIdentity{UID: saved.UID, ResourceVersion: saved.ResourceVersion}
		savedMetadata = saved.ObjectMeta
	} else {
		saved, err := s.core.ConfigMaps(s.namespace).Update(s.ctx, &corev1.ConfigMap{ObjectMeta: object, Data: map[string]string{"release": string(data)}}, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		identity = helmRecordIdentity{UID: saved.UID, ResourceVersion: saved.ResourceVersion}
		savedMetadata = saved.ObjectMeta
	}
	s.mu.Lock()
	s.seen[key] = identity
	s.recordMetadata[key] = *savedMetadata.DeepCopy()
	s.mu.Unlock()
	return nil
}
func (*helmStorage) Delete(string) (*release.Release, error) { return nil, ErrHelmReadOnly }

var _ driver.Driver = (*helmStorage)(nil)
