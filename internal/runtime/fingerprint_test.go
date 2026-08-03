package runtime

import "testing"

func TestCurrentProcessFingerprintIsStable(t *testing.T) {
	first, err := CurrentProcessFingerprint()
	if err != nil {
		t.Skipf("process fingerprint unavailable: %v", err)
	}
	second, err := CurrentProcessFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("fingerprint is not stable: %q != %q", first, second)
	}
}

func TestProcessFingerprintRejectsInvalidPID(t *testing.T) {
	if _, err := ProcessFingerprint(0); err == nil {
		t.Fatal("expected invalid PID to fail")
	}
}
