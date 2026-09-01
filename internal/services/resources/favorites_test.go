package resources

import (
	"context"
	"encoding/json"
	"testing"
)

func TestFavoriteSetValidation(t *testing.T) {
	tests := []struct {
		name    string
		set     FavoriteSet
		invalid bool
	}{
		{name: "omitted set is backward compatible", set: FavoriteSet{}, invalid: false},
		{name: "empty set", set: FavoriteSet{Version: 1, Items: []FavoriteItem{}}, invalid: false},
		{name: "valid set", set: FavoriteSet{Version: 1, Items: []FavoriteItem{{ID: "f1", Kind: "pod", Namespace: "payments", Name: "api-abc"}}}, invalid: false},
		{name: "wrong version", set: FavoriteSet{Version: 2}, invalid: true},
		{name: "too many", set: FavoriteSet{Version: 1, Items: make([]FavoriteItem, 51)}, invalid: true},
		{name: "unknown kind", set: FavoriteSet{Version: 1, Items: []FavoriteItem{{ID: "f1", Kind: "node", Namespace: "ops", Name: "worker"}}}, invalid: true},
		{name: "empty id", set: FavoriteSet{Version: 1, Items: []FavoriteItem{{ID: "", Kind: "pod", Namespace: "payments", Name: "api"}}}, invalid: true},
		{name: "invalid name characters", set: FavoriteSet{Version: 1, Items: []FavoriteItem{{ID: "f1", Kind: "pod", Namespace: "payments", Name: "API_UPPER"}}}, invalid: true},
		{name: "invalid namespace", set: FavoriteSet{Version: 1, Items: []FavoriteItem{{ID: "f1", Kind: "pod", Namespace: "-bad", Name: "api"}}}, invalid: true},
		{name: "duplicate target", set: FavoriteSet{Version: 1, Items: []FavoriteItem{
			{ID: "a", Kind: "pod", Namespace: "payments", Name: "api"},
			{ID: "b", Kind: "pod", Namespace: "payments", Name: "api"},
		}}, invalid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateFavoriteSet(test.set)
			if test.invalid && err == nil {
				t.Fatalf("expected invalid: %+v", test.set)
			}
			if !test.invalid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPreferenceRoundTripsFavorites(t *testing.T) {
	value := DefaultPreferences()
	value.Favorites.Items = []FavoriteItem{{ID: "fav-1", Kind: "secret", Namespace: "ops", Name: "store"}}

	// Put persists the normalized favorites record.
	repository := &fakePreferenceRepository{}
	service := &PreferenceService{Repository: repository, Detector: DefaultSensitiveDetector{}}
	saved, err := service.Put(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Favorites.Items) != 1 || saved.Favorites.Items[0].Kind != "secret" {
		t.Fatalf("saved favorites = %#v", saved.Favorites)
	}
	var stored FavoriteSet
	var storedJSON []byte
	found := false
	for _, record := range repository.replaced {
		if record.Key == "favorites" {
			if err := json.Unmarshal(record.ValueJSON, &stored); err != nil {
				t.Fatal(err)
			}
			storedJSON = append([]byte(nil), record.ValueJSON...)
			found = true
		}
	}
	if !found || len(stored.Items) != 1 || stored.Version != 1 || stored.Items[0].Name != "store" {
		t.Fatalf("stored favorites record = %#v found=%v", stored, found)
	}

	// Get materializes stored favorites over defaults.
	loader := &fakePreferenceRepository{records: []PreferenceRecord{{Key: "favorites", ValueJSON: storedJSON, SchemaVersion: 1}}}
	loaded, err := (&PreferenceService{Repository: loader, Detector: DefaultSensitiveDetector{}}).Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Favorites.Items) != 1 || loaded.Favorites.Items[0].Name != "store" {
		t.Fatalf("loaded favorites = %#v", loaded.Favorites)
	}
}
