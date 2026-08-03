//go:build darwin

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func processFingerprint(pid int) (string, error) {
	bootTime, err := exec.Command("/usr/sbin/sysctl", "-n", "kern.boottime").Output()
	if err != nil {
		return "", fmt.Errorf("runtime: read boot identity: %w", err)
	}
	startTime, err := exec.Command("/bin/ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", fmt.Errorf("runtime: read process start metadata: %w", err)
	}
	if strings.TrimSpace(string(startTime)) == "" {
		return "", fmt.Errorf("runtime: process start metadata is empty")
	}
	material := fmt.Sprintf("darwin:%d:%s:%s", pid, strings.TrimSpace(string(bootTime)), strings.TrimSpace(string(startTime)))
	sum := sha256.Sum256([]byte(material))
	return "darwin-" + hex.EncodeToString(sum[:]), nil
}
