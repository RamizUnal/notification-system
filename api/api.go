// Package api embeds the OpenAPI specification, the source of truth for the
// HTTP contract. The server publishes it at /openapi.yaml and /docs, and the
// console generates its TypeScript types from it.
package api

import _ "embed"

// Spec is the OpenAPI 3 document describing the REST API.
//
//go:embed openapi.yaml
var Spec []byte
