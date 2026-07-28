package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Status authenticates to the loopback endpoint and requires it to prove the
// complete identity recorded in instance.json.
func Status(ctx context.Context, runtimeDir string) (PublicInstance, error) {
	instance, err := LoadInstance(runtimeDir)
	if err != nil {
		return PublicInstance{}, err
	}
	proof, err := requestControl(ctx, http.MethodGet, statusPath, instance)
	if err != nil {
		if recovered, recoveryErr := recoverStaleState(runtimeDir); recovered {
			return PublicInstance{}, errors.Join(ErrNotRunning, recoveryErr)
		}
		return PublicInstance{}, err
	}
	if proof != instance.Public() {
		return PublicInstance{}, ErrIdentityMismatch
	}
	return proof, nil
}

// Stop requests graceful cancellation. Missing or safely proven stale state is
// treated as success so the command is idempotent.
func Stop(ctx context.Context, runtimeDir string) (PublicInstance, bool, error) {
	instance, err := LoadInstance(runtimeDir)
	if err != nil {
		if errors.Is(err, ErrNotRunning) {
			return PublicInstance{}, false, nil
		}
		return PublicInstance{}, false, err
	}
	proof, err := requestControl(ctx, http.MethodPost, stopPath, instance)
	if err != nil {
		if recovered, recoveryErr := recoverStaleState(runtimeDir); recovered {
			return PublicInstance{}, false, recoveryErr
		}
		return PublicInstance{}, false, err
	}
	if proof != instance.Public() {
		return PublicInstance{}, false, ErrIdentityMismatch
	}
	return proof, true, nil
}

func requestControl(
	ctx context.Context,
	method string,
	path string,
	instance Instance,
) (PublicInstance, error) {
	if ctx == nil {
		return PublicInstance{}, errors.New("control: request context is required")
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(instance.Port))
	request, err := http.NewRequestWithContext(
		ctx,
		method,
		"http://"+address+path,
		nil,
	)
	if err != nil {
		return PublicInstance{}, fmt.Errorf("control: create request: %w", err)
	}
	request.Header.Set(ControlTokenHeader, instance.Token)

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return PublicInstance{}, fmt.Errorf("control: endpoint unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return PublicInstance{}, fmt.Errorf(
			"control: endpoint returned %s",
			response.Status,
		)
	}

	decoder := json.NewDecoder(io.LimitReader(response.Body, maxStateBytes))
	decoder.DisallowUnknownFields()
	var proof PublicInstance
	if err := decoder.Decode(&proof); err != nil {
		return PublicInstance{}, fmt.Errorf("control: decode identity proof: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return PublicInstance{}, errors.New(
				"control: identity proof contains trailing JSON",
			)
		}
		return PublicInstance{}, fmt.Errorf(
			"control: decode trailing identity proof: %w",
			err,
		)
	}
	if err := validatePublicInstance(proof); err != nil {
		return PublicInstance{}, err
	}
	return proof, nil
}

func validatePublicInstance(instance PublicInstance) error {
	private := Instance{
		Schema:      instance.Schema,
		InstanceID:  instance.InstanceID,
		Token:       "public-proof-placeholder-token-00000000",
		PID:         instance.PID,
		Fingerprint: instance.Fingerprint,
		Port:        instance.Port,
		Protocol:    instance.Protocol,
	}
	return validateInstance(private)
}

// recoverStaleState removes state only after the OS lock is acquired. Failure
// to contact an endpoint and a stale PID are not sufficient on their own.
func recoverStaleState(runtimeDir string) (bool, error) {
	lock, err := AcquireFileLock(LockPath(runtimeDir))
	if err != nil {
		if errors.Is(err, ErrLocked) {
			return false, nil
		}
		return false, err
	}
	removeErr := RemoveInstance(runtimeDir)
	closeErr := lock.Close()
	return true, errors.Join(removeErr, closeErr)
}
