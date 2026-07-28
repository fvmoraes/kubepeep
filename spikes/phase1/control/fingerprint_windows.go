//go:build windows

package control

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/sys/windows"
)

func processFingerprint(pid int) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", fmt.Errorf("control: open process: %w", err)
	}
	defer windows.CloseHandle(handle)

	var creation windows.Filetime
	var exit windows.Filetime
	var kernel windows.Filetime
	var user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return "", fmt.Errorf("control: get process times: %w", err)
	}

	creationTicks := uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)
	material := fmt.Sprintf("%d|%d", pid, creationTicks)
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:]), nil
}
