package httpx

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"myapp/internal/shared/errs"
	"myapp/internal/shared/validate"
)

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Error renders any error as problem+json. 5xx detail is never echoed to the
// client — the trace ID is, so support can find the real error in the logs.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	kind := errs.KindOf(err)
	status := statusOf(kind)

	p := Problem{
		Type:   "about:blank",
		Title:  kind.String(),
		Status: status,
	}
	if status < 500 {
		p.Detail = err.Error()
		p.Errors = errs.FieldsOf(err)
	}
	if sc := trace.SpanContextFromContext(r.Context()); sc.HasTraceID() {
		p.TraceID = sc.TraceID().String()
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// Decode reads a JSON body into dst and validates its shape. Malformed JSON
// and failed validation both surface as errs.Invalid so handlers have one
// error path: `if err := httpx.Decode(...); err != nil { httpx.Error(...) }`.
func Decode(r *http.Request, dst any, v *validate.Validator) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errs.Wrap(errs.Invalid, "malformed request body", err)
	}
	return v.Struct(dst)
}
