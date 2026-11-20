// Package observability provides structured logging, metrics and tracing,
// plus the request-scoped context values that tie them together.
package observability

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
)

type contextKey int

const correlationIDKey contextKey = iota

// WithCorrelationID returns a copy of ctx carrying the correlation ID. The ID
// follows a request through the API, is stored with each notification and is
// picked up again by the worker that delivers it, so one ID joins every log
// line about a message across process boundaries.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

// CorrelationID returns the correlation ID carried by ctx, or "".
func CorrelationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationIDKey).(string)
	return id
}

var traceContext = propagation.TraceContext{}

// TraceParent returns the W3C traceparent header value of the span in ctx,
// or "" when ctx carries no sampled span.
func TraceParent(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	traceContext.Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

// ContextWithTraceParent returns ctx with the remote span described by a
// stored traceparent value as its parent, so work done later (for example by
// a worker) joins the trace of the request that created the notification.
func ContextWithTraceParent(ctx context.Context, traceParent string) context.Context {
	if traceParent == "" {
		return ctx
	}
	return traceContext.Extract(ctx, propagation.MapCarrier{"traceparent": traceParent})
}
