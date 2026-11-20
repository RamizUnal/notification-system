// Package migrations embeds the versioned SQL schema migrations so every
// binary (and the test suite) carries the exact schema it was built against.
//
// Files follow golang-migrate's naming: NNNNNN_description.{up,down}.sql.
// Never edit a migration that has been released; add a new one instead.
package migrations

import "embed"

// FS holds the migration files.
//
//go:embed *.sql
var FS embed.FS
