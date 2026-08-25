package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const maximumPreferenceJSONBytes = 64 << 10

var dashboardSectionIDs = []string{"summary", "problems", "restarts", "workloads", "events", "logScan", "metrics"}

type PreferencesDTO struct {
	Version   int                  `json:"version"`
	UI        UIPreferences        `json:"ui"`
	Logs      LogPreferences       `json:"logs"`
	Dashboard DashboardPreferences `json:"dashboard"`
	Filters   FilterPreferences    `json:"filters"`
}
type UIPreferences struct {
	Language string `json:"language"`
}
type LogPreferences struct {
	Wrap       bool `json:"wrap"`
	Timestamps bool `json:"timestamps"`
	TailLines  int  `json:"tailLines"`
}
type DashboardPreferences struct {
	LogScanWindow  string   `json:"logScanWindow"`
	SectionOrder   []string `json:"sectionOrder"`
	HiddenSections []string `json:"hiddenSections"`
}
type FilterPreferences struct {
	Workloads SavedFilterSet `json:"workloads"`
	Pods      SavedFilterSet `json:"pods"`
	Events    SavedFilterSet `json:"events"`
	Logs      SavedFilterSet `json:"logs"`
}
type SavedFilterSet struct {
	Version int           `json:"version"`
	Items   []SavedFilter `json:"items"`
}
type SavedFilter struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Query map[string]any `json:"query"`
}

func DefaultPreferences() PreferencesDTO {
	empty := func() SavedFilterSet { return SavedFilterSet{Version: 1, Items: []SavedFilter{}} }
	return PreferencesDTO{Version: 1, UI: UIPreferences{Language: "en"}, Logs: LogPreferences{Wrap: false, Timestamps: true, TailLines: 200}, Dashboard: DashboardPreferences{LogScanWindow: "15m", SectionOrder: append([]string(nil), dashboardSectionIDs...), HiddenSections: []string{}}, Filters: FilterPreferences{Workloads: empty(), Pods: empty(), Events: empty(), Logs: empty()}}
}

type PreferenceService struct {
	Repository PreferenceRepository
	Detector   SensitiveValueDetector
}

func (service *PreferenceService) Get(ctx context.Context) (PreferencesDTO, error) {
	defaults := DefaultPreferences()
	if service == nil || service.Repository == nil {
		return defaults, domainError(CodeFeatureUnavailable, "Preferences are unavailable.", nil)
	}
	records, err := service.Repository.Load(ctx)
	if err != nil {
		return defaults, domainError(CodeClusterUnavailable, "Preferences could not be loaded.", err)
	}
	for _, record := range records {
		if record.SchemaVersion != 1 {
			continue
		}
		if err = applyPreferenceRecord(&defaults, record); err != nil {
			continue
		}
	}
	if err = ValidatePreferences(defaults); err != nil {
		return DefaultPreferences(), nil
	}
	return defaults, nil
}

func (service *PreferenceService) Put(ctx context.Context, value PreferencesDTO) (PreferencesDTO, error) {
	if service == nil || service.Repository == nil {
		return PreferencesDTO{}, domainError(CodeFeatureUnavailable, "Preferences are unavailable.", nil)
	}
	if err := ValidatePreferences(value); err != nil {
		return PreferencesDTO{}, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return PreferencesDTO{}, fmt.Errorf("resources: encode preferences: %w", err)
	}
	if service.Detector == nil {
		return PreferencesDTO{}, domainError(CodeFeatureUnavailable, "Preference sensitivity validation is unavailable.", nil)
	}
	if service.Detector.ContainsSensitiveValue(string(encoded)) {
		return PreferencesDTO{}, domainError(CodePreferenceSensitive, "The preference contains a prohibited sensitive value.", nil)
	}
	records, err := preferenceRecords(value)
	if err != nil {
		return PreferencesDTO{}, err
	}
	if err = service.Repository.Replace(ctx, records); err != nil {
		return PreferencesDTO{}, domainError(CodeClusterUnavailable, "Preferences could not be saved.", err)
	}
	return value, nil
}

func ValidatePreferences(value PreferencesDTO) error {
	if value.Version != 1 {
		return validationError("preference version must be 1")
	}
	if value.UI.Language != "en" && value.UI.Language != "pt-BR" {
		return validationError("language has an invalid value")
	}
	if value.Logs.TailLines < 1 || value.Logs.TailLines > 2000 {
		return validationError("tailLines must be between 1 and 2000")
	}
	if !contains([]string{"15m", "30m", "1h", "4h"}, value.Dashboard.LogScanWindow) {
		return validationError("logScanWindow has an invalid value")
	}
	if err := validateExactSectionOrder(value.Dashboard.SectionOrder); err != nil {
		return err
	}
	if _, err := canonicalStrings(value.Dashboard.HiddenSections, len(dashboardSectionIDs), dashboardSectionIDs, "hidden section"); err != nil {
		return err
	}
	sets := []struct {
		name       string
		value      SavedFilterSet
		collection Collection
	}{{"workloads", value.Filters.Workloads, CollectionWorkloads}, {"pods", value.Filters.Pods, CollectionPods}, {"events", value.Filters.Events, CollectionEvents}, {"logs", value.Filters.Logs, "logs"}}
	for _, set := range sets {
		if err := validateFilterSet(set.name, set.value, set.collection); err != nil {
			return err
		}
	}
	return nil
}

func validateExactSectionOrder(values []string) error {
	if len(values) != len(dashboardSectionIDs) {
		return validationError("sectionOrder must contain every known section once")
	}
	seen := map[string]struct{}{}
	for _, value := range values {
		if !contains(dashboardSectionIDs, value) {
			return validationError("sectionOrder has an invalid section")
		}
		if _, ok := seen[value]; ok {
			return validationError("sectionOrder contains a duplicate")
		}
		seen[value] = struct{}{}
	}
	return nil
}
func validateFilterSet(name string, set SavedFilterSet, collection Collection) error {
	if set.Version != 1 {
		return validationError("filter set version must be 1")
	}
	if len(set.Items) > 50 {
		return validationError("filter set exceeds 50 items")
	}
	seen := map[string]struct{}{}
	for _, item := range set.Items {
		if item.ID == "" || len(item.ID) > 128 || !utf8.ValidString(item.ID) {
			return validationError("filter id is invalid")
		}
		if _, ok := seen[item.ID]; ok {
			return validationError("filter id must be unique")
		}
		seen[item.ID] = struct{}{}
		if count := utf8.RuneCountInString(item.Name); count < 1 || count > 80 {
			return validationError("filter name must contain 1 to 80 characters")
		}
		if err := validateSavedQuery(collection, item.Query); err != nil {
			return err
		}
	}
	encoded, _ := json.Marshal(set)
	if len(encoded) > maximumPreferenceJSONBytes {
		return validationError(name + " filters exceed 64 KiB")
	}
	return nil
}

func validateSavedQuery(collection Collection, query map[string]any) error {
	allowed := map[string]string{"namespace": "strings", "search": "string", "status": "strings", "sort": "string", "order": "string"}
	switch collection {
	case CollectionWorkloads:
		allowed["kind"] = "strings"
	case CollectionPods:
		allowed["workload"] = "string"
		allowed["node"] = "string"
		allowed["restarts"] = "string"
		allowed["problematic"] = "bool"
	case CollectionEvents:
		allowed["objectKind"] = "string"
		allowed["reason"] = "string"
	case "logs":
		delete(allowed, "status")
		delete(allowed, "sort")
		delete(allowed, "order")
	}
	for key, value := range query {
		kind, ok := allowed[key]
		if !ok || key == "continue" || key == "limit" {
			return validationError("saved filter contains a prohibited field")
		}
		switch kind {
		case "string":
			text, ok := value.(string)
			if !ok || len(text) > MaximumSearchBytes || !utf8.ValidString(text) {
				return validationError("saved filter string is invalid")
			}
		case "bool":
			if _, ok := value.(bool); !ok {
				return validationError("saved filter boolean is invalid")
			}
		case "strings":
			values, ok := stringSlice(value)
			if !ok || len(values) > MaximumNamespaces {
				return validationError("saved filter array is invalid")
			}
			for _, text := range values {
				if text == "" || len(text) > MaximumSearchBytes || !utf8.ValidString(text) {
					return validationError("saved filter array value is invalid")
				}
			}
		}
	}
	return nil
}
func stringSlice(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return values, true
	case []any:
		result := make([]string, len(values))
		for index, item := range values {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			result[index] = text
		}
		return result, true
	default:
		return nil, false
	}
}

func preferenceRecords(value PreferencesDTO) ([]PreferenceRecord, error) {
	pairs := map[string]any{"ui.language": value.UI.Language, "logs.wrap": value.Logs.Wrap, "logs.timestamps": value.Logs.Timestamps, "logs.tail_lines": value.Logs.TailLines, "dashboard.log_scan_window": value.Dashboard.LogScanWindow, "dashboard.section_order": value.Dashboard.SectionOrder, "dashboard.hidden_sections": value.Dashboard.HiddenSections, "filters.workloads": value.Filters.Workloads, "filters.pods": value.Filters.Pods, "filters.events": value.Filters.Events, "filters.logs": value.Filters.Logs}
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	records := make([]PreferenceRecord, 0, len(keys))
	for _, key := range keys {
		encoded, err := json.Marshal(pairs[key])
		if err != nil {
			return nil, fmt.Errorf("resources: encode preference %s: %w", key, err)
		}
		records = append(records, PreferenceRecord{Key: key, ValueJSON: encoded, SchemaVersion: 1})
	}
	return records, nil
}

func applyPreferenceRecord(value *PreferencesDTO, record PreferenceRecord) error {
	switch record.Key {
	case "ui.language":
		return json.Unmarshal(record.ValueJSON, &value.UI.Language)
	case "logs.wrap":
		return json.Unmarshal(record.ValueJSON, &value.Logs.Wrap)
	case "logs.timestamps":
		return json.Unmarshal(record.ValueJSON, &value.Logs.Timestamps)
	case "logs.tail_lines":
		return json.Unmarshal(record.ValueJSON, &value.Logs.TailLines)
	case "dashboard.log_scan_window":
		return json.Unmarshal(record.ValueJSON, &value.Dashboard.LogScanWindow)
	case "dashboard.section_order":
		return json.Unmarshal(record.ValueJSON, &value.Dashboard.SectionOrder)
	case "dashboard.hidden_sections":
		return json.Unmarshal(record.ValueJSON, &value.Dashboard.HiddenSections)
	case "filters.workloads":
		return json.Unmarshal(record.ValueJSON, &value.Filters.Workloads)
	case "filters.pods":
		return json.Unmarshal(record.ValueJSON, &value.Filters.Pods)
	case "filters.events":
		return json.Unmarshal(record.ValueJSON, &value.Filters.Events)
	case "filters.logs":
		return json.Unmarshal(record.ValueJSON, &value.Filters.Logs)
	default:
		return nil
	}
}

type DefaultSensitiveDetector struct{}

var sensitivePreferencePattern = regexp.MustCompile(`(?i)(-----BEGIN [A-Z ]*PRIVATE KEY-----|authorization\s*[:=]|bearer\s+[A-Za-z0-9._~+/=-]{8,}|password\s*[:=]|token\s*[:=]|client-certificate-data\s*:|client-key-data\s*:)`)

func (DefaultSensitiveDetector) ContainsSensitiveValue(value string) bool {
	return sensitivePreferencePattern.MatchString(strings.TrimSpace(value))
}
