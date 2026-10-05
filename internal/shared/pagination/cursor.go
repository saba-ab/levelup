// Package pagination provides opaque keyset cursors: (created_at, id) encoded
// base64url. Offset pagination is banned on anything unbounded — OFFSET n
// scans n rows.
package pagination

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"myapp/internal/shared/errs"
)

func EncodeCursor(ts time.Time, id string) string {
	raw := fmt.Sprintf("%d|%s", ts.UnixMicro(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeCursor(s string) (time.Time, string, error) {
	fail := func() (time.Time, string, error) {
		return time.Time{}, "", errs.New(errs.Invalid, "malformed cursor")
	}
	if s == "" {
		return fail()
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return fail()
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return fail()
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return fail()
	}
	return time.UnixMicro(micros).UTC(), parts[1], nil
}
