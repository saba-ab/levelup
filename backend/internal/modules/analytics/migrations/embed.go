// Package migrations embeds analytics' goose migrations (schema
// analytics_svc) and the typed Go seeds of its permission catalogue.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
