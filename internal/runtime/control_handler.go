package runtime

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"sync"
)

const (
	ControlStatusPath  = "/_kubepeep/control/v1/status"
	ControlStopPath    = "/_kubepeep/control/v1/stop"
	ControlTokenHeader = "X-KubePeep-Control-Token"
)

// NewControlHandler returns the browser-inaccessible process control channel.
// cancel is called at most once and only after the stop proof has been written
// and flushed.
func NewControlHandler(state InstanceStateV1, publishedHost string, cancel func()) http.Handler {
	var stopOnce sync.Once
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setControlHeaders(writer.Header())
		method := http.MethodGet
		if request.URL.Path == ControlStopPath {
			method = http.MethodPost
		} else if request.URL.Path != ControlStatusPath {
			writeControlError(writer, http.StatusNotFound)
			return
		}
		if request.Method != method {
			writer.Header().Set("Allow", method)
			writeControlError(writer, http.StatusMethodNotAllowed)
			return
		}
		if !requestFromPublishedLoopback(request, publishedHost) {
			writeControlError(writer, http.StatusForbidden)
			return
		}
		if len(request.Header.Values("Origin")) != 0 {
			writeControlError(writer, http.StatusForbidden)
			return
		}
		if request.URL.RawQuery != "" || request.URL.ForceQuery {
			writeControlError(writer, http.StatusBadRequest)
			return
		}
		hasBody, err := requestHasBody(request)
		if err != nil || hasBody {
			writeControlError(writer, http.StatusBadRequest)
			return
		}
		tokens := request.Header.Values(ControlTokenHeader)
		if len(tokens) != 1 || !controlTokenMatches(tokens[0], state.ControlToken) {
			writeControlError(writer, http.StatusUnauthorized)
			return
		}

		writer.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(writer).Encode(state.Identity()); err != nil {
			return
		}
		if request.URL.Path == ControlStopPath {
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}
			stopOnce.Do(func() {
				if cancel != nil {
					cancel()
				}
			})
		}
	})
}

func setControlHeaders(header http.Header) {
	header.Set("Content-Type", "application/json")
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
}

func writeControlError(writer http.ResponseWriter, status int) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"error": http.StatusText(status)})
}

func requestFromPublishedLoopback(request *http.Request, publishedHost string) bool {
	if request.Host != publishedHost {
		return false
	}
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func requestHasBody(request *http.Request) (bool, error) {
	if request.Body == nil || request.Body == http.NoBody {
		return false, nil
	}
	contents, err := io.ReadAll(io.LimitReader(request.Body, 1))
	return len(contents) != 0, err
}

func controlTokenMatches(candidate, expected string) bool {
	candidateDigest := sha256.Sum256([]byte(candidate))
	expectedDigest := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(candidateDigest[:], expectedDigest[:]) == 1
}
