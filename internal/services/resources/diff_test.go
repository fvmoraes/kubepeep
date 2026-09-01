package resources

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func TestExtractLastApplied(t *testing.T) {
	previous := map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "store", "namespace": "ops"}}
	raw, err := yaml.Marshal(previous)
	if err != nil {
		t.Fatal(err)
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "store",
			Namespace: "ops",
			Annotations: map[string]string{
				LastAppliedAnnotation: string(raw),
			},
		},
	}
	extracted, found, err := ExtractLastApplied(configMap)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if !strings.Contains(string(extracted), "name: store") {
		t.Fatalf("normalized previous = %s", extracted)
	}

	empty := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "store"}}
	if _, found, err := ExtractLastApplied(empty); found || err != nil {
		t.Fatalf("absent annotation: found=%v err=%v", found, err)
	}
}

func TestStripLastAppliedAnnotationKeepsOtherAnnotations(t *testing.T) {
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: "store",
		Annotations: map[string]string{
			LastAppliedAnnotation:      "{}",
			"custom.example.com/owner": "team-a",
		},
	}}
	stripped := StripLastAppliedAnnotation(configMap).(*corev1.ConfigMap)
	if _, ok := stripped.Annotations[LastAppliedAnnotation]; ok {
		t.Fatal("last-applied annotation survived stripping")
	}
	if stripped.Annotations["custom.example.com/owner"] != "team-a" {
		t.Fatalf("unrelated annotations were dropped: %#v", stripped.Annotations)
	}
}

func diffKinds(lines []DiffLineDTO) []DiffLineKind {
	kinds := make([]DiffLineKind, 0, len(lines))
	for _, line := range lines {
		kinds = append(kinds, line.Kind)
	}
	return kinds
}

func TestDiffYAMLProducesReadableOperations(t *testing.T) {
	previous := []byte("a: 1\nb: 2\nc: 3\n")
	current := []byte("a: 1\nb: 9\nd: 4\n")
	diff := DiffYAML(current, previous)
	if diff.Truncated {
		t.Fatal("small diff must not be truncated")
	}
	// A minimal edit of one line plus one insertion is 5 operations here.
	if len(diff.Lines) < 5 {
		t.Fatalf("unexpected diff size: %#v", diff.Lines)
	}
	added, removed := 0, 0
	for _, line := range diff.Lines {
		switch line.Kind {
		case DiffAdded:
			added++
		case DiffRemoved:
			removed++
		}
	}
	if added != 2 || removed != 2 {
		t.Fatalf("added=%d removed=%d lines=%#v", added, removed, diff.Lines)
	}
	if !strings.Contains(string(current), "d: 4") {
		t.Fatal("sanity")
	}
}

func TestDiffYAMLIdenticalDocumentsAreAllSame(t *testing.T) {
	document := []byte("a: 1\nb: 2\n")
	diff := DiffYAML(document, document)
	for _, line := range diff.Lines {
		if line.Kind != DiffSame {
			t.Fatalf("identical documents produced %s line: %#v", line.Kind, line)
		}
	}
	if len(diff.Lines) != 2 {
		t.Fatalf("lines = %#v", diff.Lines)
	}
}

func TestDiffYAMLTruncatesOversizedResults(t *testing.T) {
	previous := []byte(strings.Repeat("x\n", MaximumDiffLines+50))
	current := []byte(strings.Repeat("y\n", MaximumDiffLines+50))
	diff := DiffYAML(current, previous)
	if !diff.Truncated || len(diff.Lines) != MaximumDiffLines {
		t.Fatalf("truncated=%v lines=%d", diff.Truncated, len(diff.Lines))
	}
}
