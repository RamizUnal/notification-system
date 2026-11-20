// Package httpapi exposes the use cases over a versioned REST API, plus a
// WebSocket event stream, health checks, Prometheus metrics and the OpenAPI
// documentation.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/realtime"
	"github.com/RamizUnal/notification-system/internal/service"
)

// The interfaces below are what the handlers need from the application
// layer. Declaring them here keeps the handlers testable with small fakes.

// NotificationService covers notifications and batches.
type NotificationService interface {
	Create(ctx context.Context, in service.NotificationInput) (service.CreateResult, error)
	CreateBatch(ctx context.Context, in service.BatchInput) (service.BatchResult, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Notification, error)
	List(ctx context.Context, f service.ListFilter) (service.Page[domain.Notification], error)
	Cancel(ctx context.Context, id uuid.UUID) (domain.Notification, error)
	Retry(ctx context.Context, id uuid.UUID) (domain.Notification, error)
	Attempts(ctx context.Context, id uuid.UUID) ([]domain.DeliveryAttempt, error)
	Batch(ctx context.Context, id uuid.UUID) (domain.BatchSummary, error)
	CancelBatch(ctx context.Context, id uuid.UUID) (int, error)
}

// TemplateService covers message templates.
type TemplateService interface {
	Create(ctx context.Context, d domain.TemplateDraft) (domain.Template, error)
	Update(ctx context.Context, id uuid.UUID, d domain.TemplateDraft) (domain.Template, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Template, error)
	List(ctx context.Context) ([]domain.Template, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Render(ctx context.Context, id uuid.UUID, vars map[string]string) (string, error)
}

// MetricsService reports system metrics.
type MetricsService interface {
	Snapshot(ctx context.Context, window time.Duration) (service.MetricsSnapshot, error)
}

// HTTPObserver records request metrics.
type HTTPObserver interface {
	ObserveHTTP(method, route string, status int, elapsed time.Duration)
}

// Config wires the API.
type Config struct {
	Notifications NotificationService
	Templates     TemplateService
	Metrics       MetricsService
	Hub           *realtime.Hub
	// ReadinessChecks are probed by /readyz, keyed by dependency name.
	ReadinessChecks map[string]func(context.Context) error
	// PrometheusHandler serves /metrics.
	PrometheusHandler http.Handler
	HTTPObserver      HTTPObserver
	Logger            *slog.Logger
	// AllowedOrigins lists host patterns allowed to open the WebSocket from
	// another origin (e.g. a Vite dev server), such as "localhost:*".
	AllowedOrigins []string
	// OpenAPISpec is served at /openapi.yaml and rendered at /docs.
	OpenAPISpec []byte
}

// API holds the HTTP handlers.
type API struct {
	cfg    Config
	logger *slog.Logger
}

// NewRouter builds the HTTP handler with all routes and middleware.
func NewRouter(cfg Config) http.Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	a := &API{cfg: cfg, logger: cfg.Logger}

	r := chi.NewRouter()
	r.Use(correlationID)
	r.Use(a.recoverPanics)
	r.Use(a.observe)
	r.NotFound(a.handle(func(_ http.ResponseWriter, r *http.Request) error {
		return newRequestError(http.StatusNotFound, codeNotFound, "No route matches "+r.URL.Path+".")
	}))
	r.MethodNotAllowed(a.handle(func(_ http.ResponseWriter, r *http.Request) error {
		return newRequestError(http.StatusMethodNotAllowed, codeMethodNotAllowed,
			r.Method+" is not supported on "+r.URL.Path+".")
	}))

	r.Get("/healthz", a.liveness)
	r.Get("/readyz", a.readiness)
	if cfg.PrometheusHandler != nil {
		r.Method(http.MethodGet, "/metrics", cfg.PrometheusHandler)
	}
	r.Get("/openapi.yaml", a.openAPISpec)
	r.Get("/docs", a.docs)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.NoCache)

		r.Route("/notifications", func(r chi.Router) {
			r.Post("/", a.handle(a.createNotification))
			r.Get("/", a.handle(a.listNotifications))
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", a.handle(a.getNotification))
				r.Get("/attempts", a.handle(a.listAttempts))
				r.Post("/cancel", a.handle(a.cancelNotification))
				r.Post("/retry", a.handle(a.retryNotification))
			})
		})

		r.Route("/batches", func(r chi.Router) {
			r.Post("/", a.handle(a.createBatch))
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", a.handle(a.getBatch))
				r.Get("/notifications", a.handle(a.listBatchNotifications))
				r.Post("/cancel", a.handle(a.cancelBatch))
			})
		})

		r.Route("/templates", func(r chi.Router) {
			r.Post("/", a.handle(a.createTemplate))
			r.Get("/", a.handle(a.listTemplates))
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", a.handle(a.getTemplate))
				r.Put("/", a.handle(a.updateTemplate))
				r.Delete("/", a.handle(a.deleteTemplate))
				r.Post("/preview", a.handle(a.previewTemplate))
			})
		})

		r.Get("/metrics", a.handle(a.metrics))
		r.Get("/events", a.events)
	})
	return r
}

// handlerFunc is an HTTP handler that reports failure by returning an error.
type handlerFunc func(http.ResponseWriter, *http.Request) error

// handle adapts a handlerFunc to http.HandlerFunc. It is the single place
// where errors become responses: handlers return errors, and handle writes
// the matching problem details, so every endpoint fails the same way.
func (a *API) handle(fn handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := fn(w, r)
		if err == nil {
			return
		}
		if r.Context().Err() != nil {
			// The client went away; there is nobody to answer.
			return
		}
		if ww, ok := w.(middleware.WrapResponseWriter); ok && ww.Status() != 0 {
			a.logger.ErrorContext(r.Context(), "failed after the response started", slog.Any("error", err))
			return
		}

		p, expected := problemFor(err)
		p.Instance = r.URL.Path
		p.RequestID = w.Header().Get(correlationHeader)
		if !expected {
			a.logger.ErrorContext(r.Context(), "request failed",
				slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Any("error", err))
		}
		writeProblem(w, p)
	}
}
