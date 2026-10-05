// Package migrations embeds the outbox schema (outbox_svc), applied by
// cmd/migrate before any module (R3 ordering).
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
