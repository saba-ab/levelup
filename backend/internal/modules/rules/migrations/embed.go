// Package migrations embeds rules' goose migrations (schema rules_svc,
// applied by cmd/migrate in registry order, R3) and its typed Go seeds.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
