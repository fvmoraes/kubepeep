package resources

import (
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
)

func TestPodDetailRelatedEventsJSON(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		events []ResourceRef
		want   int
	}{
		{name: "absent", want: 0},
		{name: "empty", events: []ResourceRef{}, want: 0},
		{name: "present", events: []ResourceRef{{Kind: "Event", Name: "scheduled"}}, want: 1},
		{name: "bounded", events: make([]ResourceRef, 101), want: 100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := json.Marshal(PodDetail(&corev1.Pod{}, tt.events, time.Unix(200, 0)))
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &wire); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"conditions", "containers", "initContainers", "ephemeralContainers", "relatedEvents"} {
				if len(wire[field]) == 0 || wire[field][0] != '[' {
					t.Errorf("%s must serialize as an array, got %s", field, wire[field])
				}
			}
			var events []ResourceRef
			if err := json.Unmarshal(wire["relatedEvents"], &events); err != nil {
				t.Fatal(err)
			}
			if len(events) != tt.want {
				t.Errorf("events count = %d, want %d", len(events), tt.want)
			}
			if tt.want == 1 && events[0].Name != "scheduled" {
				t.Error("event reference was lost")
			}
		})
	}
}
