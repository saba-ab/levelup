// Package migrations embeds points' goose migrations (schema points_svc)
// and the typed Go seeds for its permission catalogue.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
