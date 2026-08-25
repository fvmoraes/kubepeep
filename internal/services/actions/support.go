package actions

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type cryptoIdentifiers struct{}

func (cryptoIdentifiers) NewID(prefix string) (string, error) {
	encoded, err := randomEncoded(18)
	if err != nil {
		return "", err
	}
	return prefix + encoded, nil
}

func (cryptoIdentifiers) NewToken() (string, error) { return randomEncoded(32) }

func randomEncoded(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("actions: secure identifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func copyStrings(values []string) []string { return append([]string(nil), values...) }

func safeMetadata(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
	const maximum = 256
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

var (
	ErrPortForwardPodGone   = errors.New("actions: port-forward pod gone")
	ErrExecTargetGone       = errors.New("actions: exec target gone")
	errLocalPortUnavailable = errors.New("actions: local port unavailable")
)
