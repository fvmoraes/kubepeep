package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const ControlTimeout = 2 * time.Second

var (
	ErrIdentityMismatch  = errors.New("runtime: control identity does not match instance state")
	ErrUnverifiedRuntime = errors.New("runtime: could not verify the running instance")
)

// ControlTransportError marks failures where no authenticated HTTP proof was
// received. Only these failures may participate in stale-state recovery.
type ControlTransportError struct{ Err error }

func (err *ControlTransportError) Error() string {
	return "runtime: control transport failed: " + err.Err.Error()
}
func (err *ControlTransportError) Unwrap() error { return err.Err }

// ControlProtocolError marks a response that cannot be trusted as a proof.
type ControlProtocolError struct {
	Status int
	Reason string
}

func (err *ControlProtocolError) Error() string {
	if err.Status != 0 {
		return fmt.Sprintf("runtime: control response status %d", err.Status)
	}
	return "runtime: invalid control response: " + err.Reason
}

// ControlClient performs the fixed two-second authenticated control exchange.
type ControlClient struct{}

func (ControlClient) Status(ctx context.Context, state InstanceStateV1) (ControlIdentityDTO, error) {
	return requestControl(ctx, state, http.MethodGet, ControlStatusPath)
}

func (ControlClient) Stop(ctx context.Context, state InstanceStateV1) (ControlIdentityDTO, error) {
	return requestControl(ctx, state, http.MethodPost, ControlStopPath)
}

func requestControl(parent context.Context, state InstanceStateV1, method, path string) (ControlIdentityDTO, error) {
	if err := ValidateInstanceState(state); err != nil {
		return ControlIdentityDTO{}, err
	}
	if parent == nil {
		return ControlIdentityDTO{}, errors.New("runtime: control context is required")
	}
	ctx, cancel := context.WithTimeout(parent, ControlTimeout)
	defer cancel()
	host := net.JoinHostPort("127.0.0.1", strconv.Itoa(state.Port))
	dialer := &net.Dialer{}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || address != host {
				return nil, errors.New("control dial target changed")
			}
			return dialer.DialContext(ctx, network, address)
		},
		DisableCompression: true,
		DisableKeepAlives:  true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   ControlTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://"+host+path, nil)
	if err != nil {
		return ControlIdentityDTO{}, err
	}
	request.Host = host
	request.Header.Set(ControlTokenHeader, state.ControlToken)
	response, err := client.Do(request)
	if err != nil {
		return ControlIdentityDTO{}, &ControlTransportError{Err: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, MaxStateBytes))
		return ControlIdentityDTO{}, &ControlProtocolError{Status: response.StatusCode}
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return ControlIdentityDTO{}, &ControlProtocolError{Reason: "unexpected content type"}
	}
	if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Content-Type-Options") != "nosniff" {
		return ControlIdentityDTO{}, &ControlProtocolError{Reason: "required response headers are missing"}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxStateBytes+1))
	if err != nil {
		return ControlIdentityDTO{}, &ControlProtocolError{Reason: "response body could not be read"}
	}
	if len(data) > MaxStateBytes {
		return ControlIdentityDTO{}, &ControlProtocolError{Reason: "response body exceeds 64 KiB"}
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var identity ControlIdentityDTO
	if err := decoder.Decode(&identity); err != nil {
		return ControlIdentityDTO{}, &ControlProtocolError{Reason: "response JSON is invalid"}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ControlIdentityDTO{}, &ControlProtocolError{Reason: "response has trailing JSON"}
	}
	if err := validateControlIdentity(identity); err != nil {
		return ControlIdentityDTO{}, err
	}
	if identity != state.Identity() {
		return ControlIdentityDTO{}, ErrIdentityMismatch
	}
	return identity, nil
}

func validateControlIdentity(identity ControlIdentityDTO) error {
	if identity.Schema != StateSchemaVersion || identity.Protocol != ControlProtocol || identity.PID <= 0 ||
		identity.Port < MinimumPort || identity.Port > MaximumPort || identity.Fingerprint == "" || len(identity.Fingerprint) > 256 ||
		!strings.HasPrefix(identity.InstanceID, "inst_") || !validRawURLSecret(strings.TrimPrefix(identity.InstanceID, "inst_"), 16) {
		return &ControlProtocolError{Reason: "identity fields are invalid"}
	}
	return nil
}
