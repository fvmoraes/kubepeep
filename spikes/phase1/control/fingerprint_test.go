package control

import (
	"os"
	"testing"
)

func TestCurrentProcessFingerprintIsStable(t *testing.T) {
	first, err := CurrentProcessFingerprint()
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("process metadata is unavailable: %v", err)
		}
		t.Fatalf("first fingerprint: %v", err)
	}
	second, err := CurrentProcessFingerprint()
	if err != nil {
		t.Fatalf("second fingerprint: %v", err)
	}
	if first == "" || first != second {
		t.Fatalf("fingerprints are not stable: first=%q second=%q", first, second)
	}
}

func TestProcessFingerprintRejectsInvalidPID(t *testing.T) {
	if _, err := ProcessFingerprint(0); err == nil {
		t.Fatal("ProcessFingerprint(0) succeeded")
	}
}
