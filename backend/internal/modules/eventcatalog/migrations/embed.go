// Package migrations embeds eventcatalog's goose migrations (schema
// eventcatalog_svc, applied by cmd/migrate in registry order, R3) and its
// typed Go seeds (seeds.go).
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
