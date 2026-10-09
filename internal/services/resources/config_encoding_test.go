package resources

import (
	"encoding/base64"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestConfigMapKeepsOriginalDataFieldAndEncoding(t *testing.T) {
	detail := ConvertConfigMapDetail(&corev1.ConfigMap{
		Data:       map[string]string{"text": "ação", "looks-encoded": "YcOnw6Nv"},
		BinaryData: map[string][]byte{"binary": []byte("ação")},
	})
	for _, entry := range detail.Entries {
		switch entry.Key {
		case "binary":
			if entry.Field != "binaryData" || entry.Encoding != "base64" || entry.Value != base64.StdEncoding.EncodeToString([]byte("ação")) {
				t.Fatal("binaryData was silently decoded")
			}
		case "looks-encoded":
			if entry.Field != "data" || entry.Encoding != "utf-8" || entry.Value != "YcOnw6Nv" {
				t.Fatal("data was inferred or transformed from its content")
			}
		case "text":
			if entry.Field != "data" || entry.Encoding != "utf-8" || entry.Value != "ação" {
				t.Fatal("UTF-8 data changed")
			}
		}
	}
}
