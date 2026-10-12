package resources

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func savedPortForwardFixture() SavedPortForward {
	return SavedPortForward{ClusterProfileID: 7, Context: "production", Namespace: "payments", Kind: "Pod", Name: "api-0", RemotePort: 8080, LocalPort: 18080}
}

func TestSavedPortForwardValidationRejectsInvalidRecipesBeforeStorage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*SavedPortForward)
	}{
		{"profile zero", func(value *SavedPortForward) { value.ClusterProfileID = 0 }},
		{"profile negative", func(value *SavedPortForward) { value.ClusterProfileID = -1 }},
		{"context empty", func(value *SavedPortForward) { value.Context = "" }},
		{"context whitespace", func(value *SavedPortForward) { value.Context = " \n\t" }},
		{"context too long", func(value *SavedPortForward) { value.Context = strings.Repeat("c", 1025) }},
		{"context invalid UTF8", func(value *SavedPortForward) { value.Context = string([]byte{0xff}) }},
		{"unsupported kind", func(value *SavedPortForward) { value.Kind = "Deployment" }},
		{"kind casing", func(value *SavedPortForward) { value.Kind = "pod" }},
		{"namespace empty", func(value *SavedPortForward) { value.Namespace = "" }},
		{"namespace subdomain", func(value *SavedPortForward) { value.Namespace = "payments.dev" }},
		{"namespace too long", func(value *SavedPortForward) { value.Namespace = strings.Repeat("n", 64) }},
		{"namespace uppercase", func(value *SavedPortForward) { value.Namespace = "Payments" }},
		{"name empty", func(value *SavedPortForward) { value.Name = "" }},
		{"name invalid character", func(value *SavedPortForward) { value.Name = "api/service" }},
		{"name empty label", func(value *SavedPortForward) { value.Name = "api..service" }},
		{"name invalid label", func(value *SavedPortForward) { value.Name = "api.-service" }},
		{"name too long", func(value *SavedPortForward) { value.Name = strings.Repeat("n", 254) }},
		{"local zero", func(value *SavedPortForward) { value.LocalPort = 0 }},
		{"local privileged", func(value *SavedPortForward) { value.LocalPort = 1023 }},
		{"local too large", func(value *SavedPortForward) { value.LocalPort = 65536 }},
		{"remote zero", func(value *SavedPortForward) { value.RemotePort = 0 }},
		{"remote negative", func(value *SavedPortForward) { value.RemotePort = -1 }},
		{"remote too large", func(value *SavedPortForward) { value.RemotePort = 65536 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			recipe := savedPortForwardFixture()
			test.change(&recipe)
			value := DefaultPreferences()
			value.PortForwards = []SavedPortForward{recipe}
			repository := &fakePreferenceRepository{}
			service := &PreferenceService{Repository: repository, Detector: DefaultSensitiveDetector{}}
			if _, err := service.Put(t.Context(), value); ErrorCodeOf(err) != CodeValidationFailed {
				t.Fatalf("invalid recipe error = %v", err)
			}
			if len(repository.replaced) != 0 {
				t.Fatal("invalid port-forward recipe reached storage")
			}
		})
	}
}

func TestSavedPortForwardIdentityAllowsOneRecipePerTarget(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*SavedPortForward)
		valid  bool
	}{
		{"identical target", func(*SavedPortForward) {}, false},
		{"different ports same target", func(value *SavedPortForward) { value.LocalPort++; value.RemotePort++ }, false},
		{"different profile", func(value *SavedPortForward) { value.ClusterProfileID++ }, true},
		{"different context", func(value *SavedPortForward) { value.Context = "staging" }, true},
		{"different kind", func(value *SavedPortForward) { value.Kind = "Service" }, true},
		{"different namespace", func(value *SavedPortForward) { value.Namespace = "billing" }, true},
		{"different name", func(value *SavedPortForward) { value.Name = "api-1" }, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			recipe := savedPortForwardFixture()
			other := recipe
			test.change(&other)
			value := DefaultPreferences()
			value.PortForwards = []SavedPortForward{recipe, other}
			err := ValidatePreferences(value)
			if test.valid && err != nil {
				t.Fatalf("distinct target rejected: %v", err)
			}
			if !test.valid && ErrorCodeOf(err) != CodeValidationFailed {
				t.Fatalf("duplicate target accepted: %v", err)
			}
		})
	}
}

func TestSavedPortForwardBoundariesAndServiceRoundTrip(t *testing.T) {
	t.Parallel()
	value := DefaultPreferences()
	value.PortForwards = []SavedPortForward{
		{ClusterProfileID: 1, Context: "development", Namespace: strings.Repeat("n", 63), Kind: "Pod", Name: strings.Repeat("p", 253), LocalPort: 1024, RemotePort: 1},
		{ClusterProfileID: 1, Context: "development", Namespace: "default", Kind: "Service", Name: "api.internal", LocalPort: 65535, RemotePort: 65535},
	}
	repository := &fakePreferenceRepository{}
	service := &PreferenceService{Repository: repository, Detector: DefaultSensitiveDetector{}}
	if _, err := service.Put(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	repository.records = repository.replaced
	loaded, err := service.Get(t.Context())
	if err != nil || !reflect.DeepEqual(loaded, value) {
		t.Fatalf("saved recipes changed during roundtrip: %#v / %v", loaded, err)
	}
	value.PortForwards = nil
	if _, err := service.Put(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	repository.records = repository.replaced
	loaded, err = service.Get(t.Context())
	if err != nil || len(loaded.PortForwards) != 0 {
		t.Fatalf("cleared recipes reappeared: %#v / %v", loaded.PortForwards, err)
	}
}

func TestSavedPortForwardCollectionLimits(t *testing.T) {
	t.Parallel()
	value := DefaultPreferences()
	for index := range 100 {
		recipe := savedPortForwardFixture()
		recipe.Name = fmt.Sprintf("api-%d", index)
		value.PortForwards = append(value.PortForwards, recipe)
	}
	if err := ValidatePreferences(value); err != nil {
		t.Fatalf("100 recipes rejected: %v", err)
	}
	extra := savedPortForwardFixture()
	extra.Name = "api-100"
	value.PortForwards = append(value.PortForwards, extra)
	if err := ValidatePreferences(value); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("101 recipes error = %v", err)
	}
	value.PortForwards = value.PortForwards[:100]
	encoded, err := json.Marshal(value.PortForwards)
	if err != nil {
		t.Fatal(err)
	}
	remaining := maximumPreferenceJSONBytes - len(encoded)
	for index := range value.PortForwards {
		additional := min(remaining, 1024-len(value.PortForwards[index].Context))
		value.PortForwards[index].Context += strings.Repeat("c", additional)
		remaining -= additional
	}
	encoded, err = json.Marshal(value.PortForwards)
	if err != nil || remaining != 0 || len(encoded) != maximumPreferenceJSONBytes {
		t.Fatalf("64 KiB fixture size=%d remaining=%d error=%v", len(encoded), remaining, err)
	}
	if err := ValidatePreferences(value); err != nil {
		t.Fatalf("exactly 64 KiB rejected: %v", err)
	}
	value.PortForwards[len(value.PortForwards)-1].Context += "c"
	if err := ValidatePreferences(value); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("more than 64 KiB error = %v", err)
	}
}
