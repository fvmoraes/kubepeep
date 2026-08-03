//go:build !linux && !darwin && !windows

package runtime

func processFingerprint(int) (string, error) {
	return "", ErrFingerprintUnsupported
}
