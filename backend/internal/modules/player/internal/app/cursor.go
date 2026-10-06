package app

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"levelup/internal/modules/player/contracts"
	"levelup/internal/modules/player/internal/domain"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
)

func validSort(s string) bool {
	switch s {
	case contracts.SortCreatedDesc, contracts.SortCreatedAsc, contracts.SortDisplayName:
		return true
	}
	return false
}

type listCursor struct {
	at  time.Time
	key string
	id  string
}

// sortCursor is the opaque cursor of the non-default sorts. It names its
// sort, so a cursor cannot be replayed against another order.
type sortCursor struct {
	Sort string `json:"s"`
	At   int64  `json:"t,omitempty"` // unix micros, created_at sorts
	Key  string `json:"k,omitempty"` // display_name sort
	ID   string `json:"i"`
}

var errMalformedCursor = errs.New(errs.Invalid, "malformed cursor")

// encodeListCursor keeps the shared (created_at, id) cursor for the default
// sort, so cursors issued before sorting existed stay valid.
func encodeListCursor(sortBy string, last domain.Player) string {
	if sortBy == contracts.SortCreatedDesc {
		return pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	c := sortCursor{Sort: sortBy, ID: last.ID}
	if sortBy == contracts.SortDisplayName {
		c.Key = last.SortName()
	} else {
		c.At = last.CreatedAt.UnixMicro()
	}
	b, err := json.Marshal(c)
	if err != nil { // unreachable: strings and an int64
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeListCursor(sortBy, raw string) (listCursor, error) {
	if sortBy == contracts.SortCreatedDesc {
		at, afterID, err := pagination.DecodeCursor(raw)
		if err != nil {
			return listCursor{}, err
		}
		if !isUUID(afterID) {
			return listCursor{}, errMalformedCursor
		}
		return listCursor{at: at, id: afterID}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return listCursor{}, errMalformedCursor
	}
	var c sortCursor
	if err := json.Unmarshal(b, &c); err != nil || c.Sort != sortBy || !isUUID(c.ID) {
		return listCursor{}, errMalformedCursor
	}
	return listCursor{at: time.UnixMicro(c.At).UTC(), key: c.Key, id: c.ID}, nil
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
