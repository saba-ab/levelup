// Package migrations embeds the idempotency schema (idempotency_svc).
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

var FS fs.FS = files
