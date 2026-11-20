package httpapi_test

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/api"
	"github.com/RamizUnal/notification-system/internal/httpapi"
	"github.com/RamizUnal/notification-system/internal/realtime"
)

// TestOpenAPISpecMatchesTheRouter keeps the documentation honest: the spec
// must be valid OpenAPI, and it must describe exactly the routes the server
// serves. Adding an endpoint without documenting it fails the build.
func TestOpenAPISpecMatchesTheRouter(t *testing.T) {
	t.Parallel()

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(api.Spec)
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()), "the spec is valid OpenAPI 3")

	var documented []string
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			documented = append(documented, method+" "+path)
		}
	}

	router, ok := httpapi.NewRouter(httpapi.Config{
		Hub:               realtime.NewHub(),
		PrometheusHandler: http.NotFoundHandler(),
	}).(chi.Routes)
	require.True(t, ok)

	meta := map[string]bool{"/docs": true, "/openapi.yaml": true}
	var served []string
	require.NoError(t, chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(strings.ReplaceAll(route, "/*", ""), "/")
		if route == "" {
			route = "/"
		}
		if !meta[route] {
			served = append(served, method+" "+route)
		}
		return nil
	}))

	sort.Strings(documented)
	sort.Strings(served)
	assert.Equal(t, documented, served)
}
