// Package migrations embeds webhooks' goose migrations (schema webhooks_svc)
// and the typed Go seeds of its permission catalogue.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
