package resources

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const MaximumConfigMapJSONBytes = 10 << 20

type ConfigMapListDTO struct {
	Namespace         string `json:"namespace"`
	Name              string `json:"name"`
	UID               string `json:"uid"`
	CreationTimestamp string `json:"creationTimestamp"`
}

func (ConfigMapListDTO) resourceListItem() {}

type ConfigMapEntryDTO struct {
	Key       string `json:"key"`
	Encoding  string `json:"encoding"`
	Value     string `json:"value"`
	Truncated bool   `json:"truncated"`
}

type ConfigMapDetailDTO struct {
	Metadata   ResourceMetadataDTO `json:"metadata"`
	Entries    []ConfigMapEntryDTO `json:"entries"`
	TotalBytes int64               `json:"totalBytes"`
	Truncated  bool                `json:"truncated"`
}

func (ConfigMapDetailDTO) resourceDetailItem() {}

type SecretMetadataFieldsDTO struct {
	Name              string  `json:"name"`
	Namespace         string  `json:"namespace"`
	UID               string  `json:"uid"`
	CreationTimestamp string  `json:"creationTimestamp"`
	DeletionTimestamp *string `json:"deletionTimestamp,omitempty"`
}

type SecretMetadataDTO struct {
	APIVersion string                  `json:"apiVersion"`
	Kind       string                  `json:"kind"`
	Metadata   SecretMetadataFieldsDTO `json:"metadata"`
}

func (SecretMetadataDTO) resourceListItem()   {}
func (SecretMetadataDTO) resourceDetailItem() {}

func ConvertConfigMapMetadata(value metav1.Object) ConfigMapListDTO {
	created := ""
	createdAt := value.GetCreationTimestamp()
	if !createdAt.IsZero() {
		created = createdAt.UTC().Format(time.RFC3339)
	}
	return ConfigMapListDTO{Namespace: value.GetNamespace(), Name: value.GetName(), UID: string(value.GetUID()), CreationTimestamp: created}
}

func ConvertSecretMetadata(value metav1.PartialObjectMetadata) SecretMetadataDTO {
	created := ""
	if !value.CreationTimestamp.IsZero() {
		created = value.CreationTimestamp.UTC().Format(time.RFC3339)
	}
	var deleted *string
	if value.DeletionTimestamp != nil {
		formatted := value.DeletionTimestamp.UTC().Format(time.RFC3339)
		deleted = &formatted
	}
	return SecretMetadataDTO{APIVersion: "v1", Kind: "Secret", Metadata: SecretMetadataFieldsDTO{Name: value.Name, Namespace: value.Namespace, UID: string(value.UID), CreationTimestamp: created, DeletionTimestamp: deleted}}
}

func ConvertConfigMapDetail(value *corev1.ConfigMap) ConfigMapDetailDTO {
	detail := ConfigMapDetailDTO{Metadata: ConvertMetadata(value), Entries: []ConfigMapEntryDTO{}}
	type candidate struct {
		key    string
		raw    []byte
		binary bool
	}
	candidates := make([]candidate, 0, len(value.Data)+len(value.BinaryData))
	for key, content := range value.Data {
		candidates = append(candidates, candidate{key: key, raw: []byte(content), binary: !utf8.ValidString(content)})
		detail.TotalBytes += int64(len(content))
	}
	for key, content := range value.BinaryData {
		candidates = append(candidates, candidate{key: key, raw: append([]byte(nil), content...), binary: true})
		detail.TotalBytes += int64(len(content))
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].key < candidates[j].key })
	for _, item := range candidates {
		entry := configEntry(item.key, item.raw, item.binary, false)
		trial := detail
		trial.Entries = append(append([]ConfigMapEntryDTO(nil), detail.Entries...), entry)
		encoded, _ := json.Marshal(trial)
		if len(encoded) <= MaximumConfigMapJSONBytes {
			detail.Entries = trial.Entries
			continue
		}
		detail.Truncated = true
		low, high, best := 0, len(item.raw), -1
		for low <= high {
			middle := (low + high) / 2
			candidateEntry := configEntry(item.key, item.raw[:middle], item.binary, true)
			candidateDetail := detail
			candidateDetail.Truncated = true
			candidateDetail.Entries = append(append([]ConfigMapEntryDTO(nil), detail.Entries...), candidateEntry)
			bytes, _ := json.Marshal(candidateDetail)
			if len(bytes) <= MaximumConfigMapJSONBytes {
				best = middle
				low = middle + 1
			} else {
				high = middle - 1
			}
		}
		if best >= 0 {
			detail.Entries = append(detail.Entries, configEntry(item.key, item.raw[:best], item.binary, true))
		}
		break
	}
	return detail
}

func configEntry(key string, raw []byte, binary, truncated bool) ConfigMapEntryDTO {
	if !binary {
		end := len(raw)
		for end > 0 && !utf8.Valid(raw[:end]) {
			end--
		}
		return ConfigMapEntryDTO{Key: key, Encoding: "utf-8", Value: string(raw[:end]), Truncated: truncated || end < len(raw)}
	}
	return ConfigMapEntryDTO{Key: key, Encoding: "base64", Value: base64.StdEncoding.EncodeToString(raw), Truncated: truncated}
}
