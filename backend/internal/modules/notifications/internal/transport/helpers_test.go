package transport

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func chiCtx(r *http.Request, rctx *chi.Context) context.Context {
	return context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
}
