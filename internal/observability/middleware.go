package observability

import "net/http"

// statusRecorder captures the response status so the requests counter can
// label series without buffering response bodies.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

// RequestsMiddleware counts every served request into
// kubepeep_requests_total{method,route,status}. The route label uses the
// Go 1.22+ ServeMux request pattern when available so cardinality stays
// bounded by the static route table, never by user-supplied paths.
func RequestsMiddleware(registry *Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			recorder := &statusRecorder{ResponseWriter: response, status: http.StatusOK}
			next.ServeHTTP(recorder, request)
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
