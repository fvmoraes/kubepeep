package runtime

import (
	"errors"
	"fmt"
	"os"
)

var ErrFingerprintUnsupported = errors.New("runtime: process fingerprint is unsupported")

// CurrentProcessFingerprint returns a stable opaque fingerprint for this
// process lifetime.
func CurrentProcessFingerprint() (string, error) {
	return ProcessFingerprint(os.Getpid())
}

// ProcessFingerprint identifies a PID incarnation. It is only evidence used
// in identity comparisons; it is never authority to signal that PID.
func ProcessFingerprint(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("runtime: invalid PID %d", pid)
	}
	return processFingerprint(pid)
}
