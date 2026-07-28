//go:build !windows

package control

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func processFingerprint(pid int) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("%w: %s", ErrFingerprintUnsupported, runtime.GOOS)
	}

	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", fmt.Errorf("control: read process stat: %w", err)
	}
	// Field 2 (comm) is parenthesized and may itself contain spaces or ')'.
	// Everything after the final ')' starts at field 3; starttime is field 22.
	closeIndex := strings.LastIndexByte(string(stat), ')')
	if closeIndex < 0 || closeIndex+2 >= len(stat) {
		return "", fmt.Errorf("control: malformed /proc/%d/stat", pid)
	}
	fields := strings.Fields(string(stat[closeIndex+1:]))
	const startTimeIndex = 19 // field 22 - field 3
	if len(fields) <= startTimeIndex {
		return "", fmt.Errorf("control: incomplete /proc/%d/stat", pid)
	}
	if _, err := strconv.ParseUint(fields[startTimeIndex], 10, 64); err != nil {
		return "", fmt.Errorf("control: parse process start time: %w", err)
	}

	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", fmt.Errorf("control: read boot id: %w", err)
	}
	material := fmt.Sprintf("%s|%d|%s", strings.TrimSpace(string(bootID)), pid, fields[startTimeIndex])
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:]), nil
}
