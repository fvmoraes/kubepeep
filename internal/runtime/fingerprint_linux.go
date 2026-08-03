//go:build linux

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func processFingerprint(pid int) (string, error) {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", fmt.Errorf("runtime: read process start metadata: %w", err)
	}
	closingParenthesis := strings.LastIndexByte(string(stat), ')')
	if closingParenthesis < 0 || closingParenthesis+2 >= len(stat) {
		return "", errorsNewFingerprintMetadata()
	}
	fields := strings.Fields(string(stat[closingParenthesis+2:]))
	const startTimeIndexAfterCommand = 19
	if len(fields) <= startTimeIndexAfterCommand {
		return "", errorsNewFingerprintMetadata()
	}
	if _, err := strconv.ParseUint(fields[startTimeIndexAfterCommand], 10, 64); err != nil {
		return "", errorsNewFingerprintMetadata()
	}
	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", fmt.Errorf("runtime: read boot identity: %w", err)
	}
	material := fmt.Sprintf("linux:%d:%s:%s", pid, strings.TrimSpace(string(bootID)), fields[startTimeIndexAfterCommand])
	sum := sha256.Sum256([]byte(material))
	return "linux-" + hex.EncodeToString(sum[:]), nil
}

func errorsNewFingerprintMetadata() error {
	return fmt.Errorf("runtime: invalid process start metadata")
}
