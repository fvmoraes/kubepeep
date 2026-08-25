package dashboard

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTextDetectorClosedKeywordsBoundariesAndPriority(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		line   string
		match  bool
		reason LogReasonCode
	}{
		{"ordinary message", false, ""},
		{"terror only", false, ""},
		{"timeouted is not a bounded word", false, ""},
		{"request ERROR: failed", true, LogErrorKeyword},
		{"error followed by panic", true, LogPanic},
		{"worker oom killed", true, LogOOM},
		{"segmentation fault (core dumped)", true, LogErrorKeyword},
	} {
		t.Run(test.line, func(t *testing.T) {
			got, ok := DetectLogLine([]byte(test.line), false)
			if ok != test.match {
				t.Fatalf("match = %v, want %v (%+v)", ok, test.match, got)
			}
			if ok && got.ReasonCode != test.reason {
				t.Fatalf("reason = %q, want %q", got.ReasonCode, test.reason)
			}
		})
	}
}

func TestJSONDetectorTypesPriorityExcerptAndTimestamp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		line    string
		match   bool
		reason  LogReasonCode
		excerpt string
	}{
		{"panic message wins all", `{"message":"panic now","stack":"trace","level":"error","error":true}`, true, LogPanic, "panic now"},
		{"stack wins level", `{"stack":"trace","level":"fatal"}`, true, LogStackTrace, "trace"},
		{"level wins error", `{"severity":"critical","error":true}`, true, LogJSONErrorLevel, "critical"},
		{"level before severity", `{"level":"error","severity":"fatal"}`, true, LogJSONErrorLevel, "error"},
		{"message before msg", `{"message":"failed one","msg":"failed two"}`, true, LogErrorKeyword, "failed one"},
		{"error string", `{"error":" actual "}`, true, LogJSONErrorField, " actual "},
		{"error true", `{"error":true}`, true, LogJSONErrorField, "true"},
		{"error number", `{"error":2}`, true, LogJSONErrorField, "2"},
		{"error array", `{"error":["x"]}`, true, LogJSONErrorField, `["x"]`},
		{"canonical error object", `{"error":{"z":1,"a":2}}`, true, LogJSONErrorField, `{"a":2,"z":1}`},
		{"error null", `{"error":null}`, false, "", ""},
		{"error false", `{"error":false}`, false, "", ""},
		{"error zero", `{"error":0}`, false, "", ""},
		{"error whitespace", `{"error":"   "}`, false, "", ""},
		{"error empty array", `{"error":[]}`, false, "", ""},
		{"error empty object", `{"error":{}}`, false, "", ""},
		{"nested field ignored", `{"nested":{"level":"error"}}`, false, "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := DetectLogLine([]byte(test.line), false)
			if ok != test.match {
				t.Fatalf("match = %v, want %v (%+v)", ok, test.match, got)
			}
			if ok && (got.ReasonCode != test.reason || got.Excerpt != test.excerpt) {
				t.Fatalf("got reason/excerpt %q/%q, want %q/%q", got.ReasonCode, got.Excerpt, test.reason, test.excerpt)
			}
		})
	}

	line := `2026-08-10T11:00:00-03:00 {"timestamp":"2026-08-10T15:00:00Z","message":"error now"}`
	got, ok := DetectLogLine([]byte(line), false)
	if !ok || got.Timestamp == nil || *got.Timestamp != "2026-08-10T15:00:00Z" {
		t.Fatalf("JSON timestamp did not override stream timestamp: %+v", got)
	}
}

func TestDetectorRejectsTrailingJSONAndDepthOverEightAsJSONObject(t *testing.T) {
	t.Parallel()
	if _, ok := decodeJSONObject([]byte(`{"level":"info"} {"level":"fatal"}`)); ok {
		t.Fatal("trailing JSON was accepted as a single object")
	}
	deep := `{"a":{"b":{"c":{"d":{"e":{"f":{"g":{"h":{"level":"fatal"}}}}}}}}}`
	if _, ok := decodeJSONObject([]byte(deep)); ok {
		t.Fatal("object deeper than eight was inspected")
	}
}

func TestRedactionSensitiveClasses(t *testing.T) {
	t.Parallel()
	jwt := strings.Repeat("a", 24) + "." + strings.Repeat("b", 24) + "." + strings.Repeat("c", 16)
	tests := []string{
		"error Authorization: Bearer abcdefghijklmnop",
		"error Authorization: Basic dXNlcjpwYXNzd29yZA== trailing-secret",
		`error Authorization: "Bearer abcdefghijklmnop" trailing-secret`,
		"error bearer abcdefghijklmnop",
		"error bearer abc",
		"error basic eA==",
		"error jwt=" + jwt,
		`error {"password":"super-secret"}`,
		`error {"password":"secret with \"quoted\" tail","message":"kept"}`,
		"error password=super-secret",
		`error password = "secret with spaces" kept-suffix`,
		`error passwd='secret with spaces' kept-suffix`,
		"error postgres://user:super-secret@db.local/app",
		`error postgres:\/\/user:super-secret@db.local\/app`,
		"error X-Api-Key: abcdefghijklmnop",
		"error -----BEGIN PRIVATE KEY-----",
		"error -----BEGIN PRIVATE KEY-----\nbase64-secret-material\n-----END PRIVATE KEY----- trailing",
		"error " + "ghp_" + "abcdefghijklmnopqrstuvwxyz123456",
		"error " + "AKIA" + "ABCDEFGHIJKLMNOP",
	}
	for _, line := range tests {
		match, ok := DetectLogLine([]byte(line), false)
		if !ok || !match.Redacted {
			t.Fatalf("line was not matched/redacted: %q (%+v)", line, match)
		}
		for _, secret := range []string{"super-secret", "secret with", "quoted", "abcdefghijklmnop", "dXNlcjpwYXNzd29yZA==", "trailing-secret", "base64-secret-material", jwt, "AKIA" + "ABCDEFGHIJKLMNOP"} {
			if strings.Contains(match.Excerpt, secret) {
				t.Fatalf("secret %q remained in %q", secret, match.Excerpt)
			}
		}
	}
}

func TestRedactionQuotedAuthorizationAndCompletePrivateKeyBlocks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		secrets []string
		kept    string
	}{
		{
			name:    "quoted JSON authorization with escaped quote",
			input:   `error {"Authorization":"Bearer credential-with-\"escaped-tail","message":"keep-this-field"}`,
			secrets: []string{`credential-with-\"escaped-tail`},
			kept:    "keep-this-field",
		},
		{
			name:    "standalone quoted bearer",
			input:   `error Bearer "credential with spaces and \"quoted\" text" keep-this-suffix`,
			secrets: []string{`credential with spaces`, `quoted`},
			kept:    "keep-this-suffix",
		},
		{
			name:    "standalone single quoted basic",
			input:   `error Basic 'dXNlcjpwYXNzd29yZA==' keep-basic-suffix`,
			secrets: []string{"dXNlcjpwYXNzd29yZA=="},
			kept:    "keep-basic-suffix",
		},
		{
			name: "complete encrypted PEM block",
			input: "prefix\n-----BEGIN ENCRYPTED PRIVATE KEY-----\n" +
				"error private-body-material\n" +
				"-----END ENCRYPTED PRIVATE KEY-----\nsuffix-after-block",
			secrets: []string{"private-body-material", "BEGIN ENCRYPTED PRIVATE KEY", "END ENCRYPTED PRIVATE KEY"},
			kept:    "suffix-after-block",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			redacted, changed := Redact(test.input)
			if !changed {
				t.Fatalf("sensitive value was not redacted: %q", test.input)
			}
			for _, secret := range test.secrets {
				if strings.Contains(redacted, secret) {
					t.Fatalf("secret %q remained in %q", secret, redacted)
				}
			}
			if !strings.Contains(redacted, test.kept) {
				t.Fatalf("non-secret suffix %q was removed from %q", test.kept, redacted)
			}
		})
	}
}

func TestExcerptTruncatesOnUTF8BoundaryAfterRedaction(t *testing.T) {
	t.Parallel()
	line := "error " + strings.Repeat("界", MaximumExcerptBytes)
	match, ok := DetectLogLine([]byte(line), true)
	if !ok || !match.Truncated || len(match.Excerpt) > MaximumExcerptBytes || !utf8.ValidString(match.Excerpt) {
		t.Fatalf("invalid truncation result: bytes=%d match=%+v", len(match.Excerpt), match)
	}
}

func TestStreamTimestampFallback(t *testing.T) {
	t.Parallel()
	match, ok := DetectLogLine([]byte("2026-08-10T12:00:00Z error happened"), false)
	if !ok || match.Timestamp == nil {
		t.Fatalf("missing stream timestamp: %+v", match)
	}
	parsed, err := time.Parse(time.RFC3339Nano, *match.Timestamp)
	if err != nil || !parsed.Equal(time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("invalid stream timestamp %v/%v", match.Timestamp, err)
	}
}
