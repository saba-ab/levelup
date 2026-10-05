package httpx

import (
	"net/http"

	"levelup/internal/shared/errs"
)

// statusOf is the single place HTTP learns about error kinds. The domain
// carries no HTTP knowledge (PRD §5: errs has "no HTTP knowledge").
func statusOf(k errs.Kind) int {
	switch k {
	case errs.Invalid:
		return http.StatusUnprocessableEntity
	case errs.NotFound:
		return http.StatusNotFound
	case errs.AlreadyExists, errs.Conflict:
		return http.StatusConflict
	case errs.PermissionDenied:
		return http.StatusForbidden
	case errs.Unauthenticated:
		return http.StatusUnauthorized
	case errs.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// Problem is RFC 9457 problem+json.
type Problem struct {
	Type    string            `json:"type"`
	Title   string            `json:"title"`
	Status  int               `json:"status"`
	Detail  string            `json:"detail,omitempty"`
	Code    string            `json:"code,omitempty"`
	Errors  map[string]string `json:"errors,omitempty"`
	TraceID string            `json:"trace_id,omitempty"`
}
