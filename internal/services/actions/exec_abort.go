package actions

import (
	"crypto/subtle"
	"net/http"
)

// ExecAbortReason is intentionally closed: the WebSocket bridge cannot invent
// public errors or close codes for an active exec session.
type ExecAbortReason string

const (
	ExecAbortProtocolViolation ExecAbortReason = "protocol_violation"
	ExecAbortBackpressure      ExecAbortReason = "backpressure"
	ExecAbortMessageTooLarge   ExecAbortReason = "message_too_large"
	ExecAbortInternal          ExecAbortReason = "internal"
)

// Abort terminates an active exec with one of the transport failures owned by
// the local WebSocket bridge. Remote and lifecycle failures remain owned by
// the exec manager itself.
func (m *ExecManager) Abort(sessionID string, reason ExecAbortReason) error {
	if err := validateSessionID(sessionID, "exec_"); err != nil {
		return err
	}
	var terminal ExecTerminal
	switch reason {
	case ExecAbortProtocolViolation:
		terminal = terminalError(CodeProtocolViolation, false, 1008)
	case ExecAbortBackpressure:
		terminal = terminalError(CodeLimitExceeded, true, 1008)
	case ExecAbortMessageTooLarge:
		terminal = terminalError(CodeLimitExceeded, false, 1009)
	case ExecAbortInternal:
		terminal = terminalError(CodeInternal, false, 1011)
	default:
		return validationError(FieldViolation{Field: "reason", Rule: "supported_exec_abort_reason"})
	}
	m.mu.Lock()
	state, ok := m.sessions[sessionID]
	if !ok || !state.active {
		m.mu.Unlock()
		return publicError(CodeSessionGone, http.StatusGone, false, nil)
	}
	m.mu.Unlock()
	m.requestTerminal(state, terminal)
	return nil
}

// ReleaseUpgrade drops a consumed-ticket reservation when the local
// WebSocket handshake cannot be completed. The opaque lease ID prevents a
// stale handler from releasing a later session with the same public ID.
func (m *ExecManager) ReleaseUpgrade(grant ExecGrant) error {
	m.mu.Lock()
	state, ok := m.sessions[grant.SessionID]
	if !ok || state.active || state.leaseID == "" || subtle.ConstantTimeCompare([]byte(state.leaseID), []byte(grant.leaseID)) != 1 {
		m.mu.Unlock()
		return publicError(CodeSessionGone, http.StatusGone, false, nil)
	}
	delete(m.sessions, grant.SessionID)
	state.reservationTimer.Stop()
	m.mu.Unlock()
	close(state.terminal)
	return nil
}
