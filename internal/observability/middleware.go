package observability

import (
	"bufio"
	"net"
	"net/http"
)

// statusRecorder captures the response status so the requests counter can
// label series without buffering response bodies.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (recorder *statusRecorder) WriteHeader(status int) {
	if !recorder.wroteHeader && (status >= 200 || status == http.StatusSwitchingProtocols) {
		recorder.status = status
		recorder.wroteHeader = true
	}
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(body []byte) (int, error) {
	if !recorder.wroteHeader {
		recorder.WriteHeader(http.StatusOK)
	}
	return recorder.ResponseWriter.Write(body)
}

func (recorder *statusRecorder) Unwrap() http.ResponseWriter { return recorder.ResponseWriter }

type recordingFlusher struct {
	recorder *statusRecorder
	flusher  http.Flusher
}

func (writer recordingFlusher) Flush() {
	if !writer.recorder.wroteHeader {
		writer.recorder.WriteHeader(http.StatusOK)
	}
	writer.flusher.Flush()
}

type recordingHijacker struct {
	recorder *statusRecorder
	hijacker http.Hijacker
}

func (writer recordingHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	connection, buffer, err := writer.hijacker.Hijack()
	if err == nil && !writer.recorder.wroteHeader {
		writer.recorder.status = http.StatusSwitchingProtocols
		writer.recorder.wroteHeader = true
	}
	return connection, buffer, err
}

// Preserve the streaming capabilities of the underlying server writer.
func (recorder *statusRecorder) streamingWriter() http.ResponseWriter {
	flusher, flushOK := recorder.ResponseWriter.(http.Flusher)
	hijacker, hijackOK := recorder.ResponseWriter.(http.Hijacker)
	switch {
	case flushOK && hijackOK:
		return struct {
			*statusRecorder
			http.Flusher
			http.Hijacker
		}{recorder, recordingFlusher{recorder, flusher}, recordingHijacker{recorder, hijacker}}
	case flushOK:
		return struct {
			*statusRecorder
			http.Flusher
		}{recorder, recordingFlusher{recorder, flusher}}
	case hijackOK:
		return struct {
			*statusRecorder
			http.Hijacker
		}{recorder, recordingHijacker{recorder, hijacker}}
	default:
		return recorder
	}
}

// RequestsMiddleware counts every served request into
// kubepeep_requests_total{method,route,status}. The route label uses the
// Go 1.22+ ServeMux request pattern when available so cardinality stays
// bounded by the static route table, never by user-supplied paths.
func RequestsMiddleware(registry *Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			recorder := &statusRecorder{ResponseWriter: response, status: http.StatusOK}
			next.ServeHTTP(recorder.streamingWriter(), request)
			route := request.Pattern
			if route == "" {
				route = "unmatched"
			}
			registry.IncCounter(RequestsTotalName, map[string]string{
				"method": request.Method,
				"route":  route,
				"status": itoa(recorder.status),
			})
		})
	}
}

func itoa(value int) string {
	digits := "0123456789"
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buffer [20]byte
	position := len(buffer)
	for value > 0 {
		position--
		buffer[position] = digits[value%10]
		value /= 10
	}
	if negative {
		position--
		buffer[position] = '-'
	}
	return string(buffer[position:])
}
