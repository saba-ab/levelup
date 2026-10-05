// Package migrations embeds program's goose migrations (schema program_svc)
// and the typed Go seeds of its permission catalogue.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
