package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/RamizUnal/notification-system/internal/observability"
)

const correlationHeader = "X-Correlation-ID"

// correlationID takes the caller's X-Correlation-ID (or X-Request-ID) if it
// is reasonable, or mints one, then puts it in the request context and the
// response headers. The ID is stored with every notification the request
// creates, so the worker's log lines carry it too.
func correlationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(correlationHeader)
		if id == "" {
			id = r.Header.Get("X-Request-ID")
		}
		if !validCorrelationID(id) {
			id = uuid.NewString()
		}
		w.Header().Set(correlationHeader, id)
		next.ServeHTTP(w, r.WithContext(observability.WithCorrelationID(r.Context(), id)))
	})
}

func validCorrelationID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for i := range len(id) {
		if id[i] < '!' || id[i] > '~' {
			return false
		}
	}
	return true
}

// recoverPanics turns a panic into a logged 500 instead of a dropped
// connection, and keeps the server running.
func (a *API) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec) // a deliberate abort: let net/http drop the connection
			}
			a.logger.ErrorContext(r.Context(), "panic while serving request",
				slog.Any("panic", rec), slog.String("stack", string(debug.Stack())),
				slog.String("method", r.Method), slog.String("path", r.URL.Path))
			p := newProblem(http.StatusInternalServerError, codeInternal,
				"Something went wrong on our side. Quote the request_id when reporting it.", nil)
			p.Instance = r.URL.Path
			p.RequestID = w.Header().Get(correlationHeader)
			writeProblem(w, p)
		}()
		next.ServeHTTP(w, r)
	})
}

// quietPaths are probed constantly by orchestrators and scrapers; logging
// them would drown the useful lines.
var quietPaths = map[string]bool{"/healthz": true, "/readyz": true, "/metrics": true}

// observe logs each request, records its latency by route pattern (not raw
// path, which would explode metric cardinality) and names the trace span
// after the route.
func (a *API) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		elapsed := time.Since(start)

		trace.SpanFromContext(r.Context()).SetName(r.Method + " " + route)
		if a.cfg.HTTPObserver != nil {
			a.cfg.HTTPObserver.ObserveHTTP(r.Method, route, status, elapsed)
		}
		if quietPaths[r.URL.Path] {
			return
		}

		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}
		a.logger.LogAttrs(r.Context(), level, "http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Int("bytes", ww.BytesWritten()),
			slog.Float64("duration_ms", float64(elapsed.Microseconds())/1000),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		)
	})
}
