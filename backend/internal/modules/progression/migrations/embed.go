// Package migrations embeds progression's goose migrations (schema
// progression_svc) and its typed Go seeds.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
