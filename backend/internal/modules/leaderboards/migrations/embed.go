// Package migrations embeds leaderboards' goose migrations (schema
// leaderboards_svc) and the typed permission seeds.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
