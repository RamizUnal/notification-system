package httpapi

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"
)

// liveness handles GET /healthz: the process is up and serving. It checks
// no dependencies, so a database outage never gets healthy pods restarted.
func (a *API) liveness(w http.ResponseWriter, _ *http.Request) {
	_ = writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readiness handles GET /readyz: every dependency answers within two
// seconds. Load balancers stop routing to an instance that is not ready.
func (a *API) readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		checks = make(map[string]string, len(a.cfg.ReadinessChecks))
		ready  = true
	)
	for _, name := range slices.Sorted(maps.Keys(a.cfg.ReadinessChecks)) {
		check := a.cfg.ReadinessChecks[name]
		wg.Go(func() {
			err := check(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				ready = false
				checks[name] = "unavailable"
				a.logger.WarnContext(r.Context(), "readiness check failed", "dependency", name, "error", err)
				return
			}
			checks[name] = "ok"
		})
	}
	wg.Wait()

	status, code := "ready", http.StatusOK
	if !ready {
		status, code = "unavailable", http.StatusServiceUnavailable
	}
	_ = writeJSON(w, code, map[string]any{"status": status, "checks": checks})
}

// metrics handles GET /api/v1/metrics?window=5m: a JSON snapshot of queue
// depth, delivery outcomes, success rate, latency percentiles, throughput
// over time and worker health, computed from the database.
func (a *API) metrics(w http.ResponseWriter, r *http.Request) error {
	var window time.Duration
	if v := r.URL.Query().Get("window"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			var errs queryErrors
			errs.add("window", "must be a duration such as 5m or 1h")
			return errs.err()
		}
		window = d
	}
	snapshot, err := a.cfg.Metrics.Snapshot(r.Context(), window)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, toMetricsResponse(snapshot))
}

// openAPISpec handles GET /openapi.yaml.
func (a *API) openAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(a.cfg.OpenAPISpec)
}

// docs handles GET /docs with an interactive Swagger UI for the spec.
func (a *API) docs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, swaggerPage)
}

const swaggerPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Notification System API</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui.css">
  <style>body { margin: 0; } .topbar { display: none; }</style>
</head>
<body>
  <div id="swagger"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/openapi.yaml",
      dom_id: "#swagger",
      deepLinking: true,
      tryItOutEnabled: true,
      displayRequestDuration: true,
    });
  </script>
</body>
</html>`
