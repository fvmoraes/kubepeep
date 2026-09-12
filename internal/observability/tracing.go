package observability

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/fvmoraes/kubepeep/internal/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Tracing owns a process-local provider. No global SDK or environment-based
// resource detection is installed, so contexts from other instances stay apart.
type Tracing struct {
	provider  *sdktrace.TracerProvider
	transport *http.Transport
}
type tracingKey struct{}

type safeExporter struct {
	sdktrace.SpanExporter
	metrics *Registry
	report  func()
}

func (exporter safeExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if err := exporter.SpanExporter.ExportSpans(ctx, spans); err != nil {
		exporter.metrics.IncCounter(TraceExportErrorsTotalName, nil)
		if exporter.report != nil {
			exporter.report()
		}
		// SDK's default error handler writes raw endpoint errors to stderr.
		// Report through the sanitized local logger and metric instead.
	}
	return nil
}

func NewTracing(ctx context.Context, options config.OTelConfig, metrics *Registry, report func()) (*Tracing, error) {
	if !options.Enabled {
		return nil, nil
	}
	if err := config.ValidateOTel(options); err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(*options.Endpoint),
		otlptracehttp.WithHeaders(map[string]string{}),
		otlptracehttp.WithCompression(otlptracehttp.NoCompression),
		otlptracehttp.WithTimeout(2*time.Second),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}),
		otlptracehttp.WithHTTPClient(client),
	)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, errors.New("observability: trace exporter initialization failed")
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "kubepeep"))),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithBatcher(safeExporter{SpanExporter: exporter, metrics: metrics, report: report}, sdktrace.WithMaxQueueSize(1024), sdktrace.WithMaxExportBatchSize(128), sdktrace.WithBatchTimeout(time.Second), sdktrace.WithExportTimeout(2*time.Second)),
	)
	return &Tracing{provider: provider, transport: transport}, nil
}

func (tracing *Tracing) Shutdown(ctx context.Context) error {
	if tracing == nil {
		return nil
	}
	if tracing.transport != nil {
		defer tracing.transport.CloseIdleConnections()
	}
	if err := tracing.provider.Shutdown(ctx); err != nil {
		return errors.New("observability: trace exporter shutdown failed")
	}
	return nil
}

func WithTracing(ctx context.Context, tracing *Tracing) context.Context {
	if tracing == nil {
		return ctx
	}
	return context.WithValue(ctx, tracingKey{}, tracing)
}

// DetachedTracing carries only the provider into a shared background worker.
// Request cancellation and arbitrary request values must not outlive a caller.
func DetachedTracing(ctx context.Context) context.Context {
	tracing, _ := ctx.Value(tracingKey{}).(*Tracing)
	return WithTracing(context.Background(), tracing)
}

func (tracing *Tracing) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithTracing(r.Context(), tracing)))
	})
}

var spanNames = map[string]struct{}{
	"resources.list": {}, "resources.list.global": {}, "resources.list.fanout": {}, "resources.list.origin": {},
	"resources.merge": {}, "cursor.get": {}, "cursor.put": {}, "watch.connect": {}, "watch.reconnect": {},
	"cache.authorization": {}, "cache.clients": {}, "cache.snapshot": {}, "cache.apply_event": {},
}

// SafeSpanAttributes is deliberately numeric except for the closed strategy
// vocabulary. It cannot carry names, UIDs, selectors, tokens, or identities.
type SafeSpanAttributes struct {
	Strategy        string
	NamespaceCount  int
	PageSize        int
	OriginChunkSize int
	Fanout          int
	ItemsCount      int
}

func safeSpanAttributes(values SafeSpanAttributes) []attribute.KeyValue {
	attributes := make([]attribute.KeyValue, 0, 6)
	if values.Strategy == "global" || values.Strategy == "fanout" {
		attributes = append(attributes, attribute.String("strategy", values.Strategy))
	}
	for key, value := range map[string]int{
		"namespace_count":   values.NamespaceCount,
		"page_size":         values.PageSize,
		"origin_chunk_size": values.OriginChunkSize,
		"fanout":            values.Fanout,
		"items_count":       values.ItemsCount,
	} {
		if value > 0 {
			attributes = append(attributes, attribute.Int(key, value))
		}
	}
	return attributes
}

// CacheOutcome records a closed vocabulary, never the cache key or its value.
func CacheOutcome(ctx context.Context, outcome string) {
	switch outcome {
	case "hit", "miss", "coalesced", "refresh":
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("cache.outcome", outcome))
	}
}

func StartSpan(ctx context.Context, name string) (context.Context, func(error)) {
	return StartSpanWithAttributes(ctx, name, SafeSpanAttributes{})
}

func StartSpanWithAttributes(ctx context.Context, name string, values SafeSpanAttributes) (context.Context, func(error)) {
	tracing, _ := ctx.Value(tracingKey{}).(*Tracing)
	if tracing == nil {
		return ctx, func(error) {}
	}
	if _, ok := spanNames[name]; !ok {
		return ctx, func(error) {}
	}
	options := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindInternal)}
	if attributes := safeSpanAttributes(values); len(attributes) > 0 {
		options = append(options, trace.WithAttributes(attributes...))
	}
	ctx, span := tracing.provider.Tracer("kubepeep").Start(ctx, name, options...)
	return ctx, func(err error) {
		if err != nil {
			span.SetStatus(codes.Error, "operation failed")
		}
		span.End()
	}
}
