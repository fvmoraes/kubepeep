package control

import (
	"errors"
	"os"
)

// ErrFingerprintUnsupported reports that the current operating system does
// not expose a process creation fingerprint through this isolated spike.
var ErrFingerprintUnsupported = errors.New("control: process fingerprint unsupported")

// CurrentProcessFingerprint returns a stable fingerprint for the lifetime of
// the current process.
func CurrentProcessFingerprint() (string, error) {
	return ProcessFingerprint(os.Getpid())
}

// ProcessFingerprint returns a fingerprint derived from OS process-creation
// metadata. It is evidence carried in the authenticated control handshake; it
// is never authority to signal or terminate a PID.
func ProcessFingerprint(pid int) (string, error) {
	if pid <= 0 {
		return "", os.ErrInvalid
	}
	return processFingerprint(pid)
}
