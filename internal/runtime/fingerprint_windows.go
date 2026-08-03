//go:build windows

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/sys/windows"
)

func processFingerprint(pid int) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", fmt.Errorf("runtime: open process for fingerprint: %w", err)
	}
	defer windows.CloseHandle(handle)
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return "", fmt.Errorf("runtime: read process creation time: %w", err)
	}
	material := fmt.Sprintf("windows:%d:%d:%d", pid, creation.HighDateTime, creation.LowDateTime)
	sum := sha256.Sum256([]byte(material))
	return "windows-" + hex.EncodeToString(sum[:]), nil
}
