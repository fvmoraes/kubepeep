package resources

import (
	"context"
	"errors"
	"testing"
)

type fakePreferenceRepository struct {
	records    []PreferenceRecord
	replaced   []PreferenceRecord
	loadErr    error
	replaceErr error
}

func (fake *fakePreferenceRepository) Load(context.Context) ([]PreferenceRecord, error) {
	return append([]PreferenceRecord(nil), fake.records...), fake.loadErr
}
func (fake *fakePreferenceRepository) Replace(_ context.Context, records []PreferenceRecord) error {
	fake.replaced = append([]PreferenceRecord(nil), records...)
	return fake.replaceErr
}

func TestPreferenceGetMaterializesDefaultsAndIsolatesFutureRecords(t *testing.T) {
	repository := &fakePreferenceRepository{records: []PreferenceRecord{{Key: "ui.language", ValueJSON: []byte(`"pt-BR"`), SchemaVersion: 1}, {Key: "logs.tail_lines", ValueJSON: []byte(`9999`), SchemaVersion: 2}, {Key: "unknown", ValueJSON: []byte(`"ignored"`), SchemaVersion: 1}}}
	service := &PreferenceService{Repository: repository}
	value, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if value.UI.Language != "pt-BR" || value.Logs.TailLines != 200 || value.Filters.Pods.Items == nil {
		t.Fatalf("preferences = %#v", value)
	}
}

func TestPreferencePutValidatesAndReplacesAllKeysTransactionally(t *testing.T) {
	repository := &fakePreferenceRepository{}
	service := &PreferenceService{Repository: repository, Detector: DefaultSensitiveDetector{}}
	value := DefaultPreferences()
	value.UI.Language = "pt-BR"
	value.Filters.Pods.Items = []SavedFilter{{ID: "problematic", Name: "Problematic", Query: map[string]any{"namespace": []any{"payments"}, "problematic": true, "status": []any{"Running"}}}}
	saved, err := service.Put(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	if saved.UI.Language != "pt-BR" || len(repository.replaced) != 11 {
		t.Fatalf("saved=%#v records=%d", saved, len(repository.replaced))
	}
	for index := 1; index < len(repository.replaced); index++ {
		if repository.replaced[index-1].Key >= repository.replaced[index].Key {
			t.Fatalf("records not canonical: %#v", repository.replaced)
		}
	}
}

func TestPreferencePutRejectsSensitiveOrArbitraryFilterBeforeStorage(t *testing.T) {
	repository := &fakePreferenceRepository{}
	service := &PreferenceService{Repository: repository, Detector: DefaultSensitiveDetector{}}
	value := DefaultPreferences()
	value.Filters.Logs.Items = []SavedFilter{{ID: "x", Name: "x", Query: map[string]any{"search": "Authorization: Bearer abcdefghijklmnop"}}}
	if _, err := service.Put(context.Background(), value); ErrorCodeOf(err) != CodePreferenceSensitive {
		t.Fatalf("sensitive error = %v", err)
	}
	value = DefaultPreferences()
	value.Filters.Pods.Items = []SavedFilter{{ID: "x", Name: "x", Query: map[string]any{"continue": "cursor"}}}
	if _, err := service.Put(context.Background(), value); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("arbitrary field = %v", err)
	}
	if len(repository.replaced) != 0 {
		t.Fatal("invalid preference reached repository")
	}
}

func TestPreferenceReplaceFailureIsSanitized(t *testing.T) {
	repository := &fakePreferenceRepository{replaceErr: errors.New("sqlite path and token")}
	service := &PreferenceService{Repository: repository, Detector: DefaultSensitiveDetector{}}
	_, err := service.Put(context.Background(), DefaultPreferences())
	if ErrorCodeOf(err) != CodeClusterUnavailable || PublicMessage(err) == repository.replaceErr.Error() {
		t.Fatalf("error = %v", err)
	}
}
