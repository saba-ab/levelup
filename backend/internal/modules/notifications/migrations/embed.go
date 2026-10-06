// Package migrations embeds notifications' goose migrations (schema notifications_svc,
// applied by cmd/migrate in registry order, R3) and its typed Go seeds.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
