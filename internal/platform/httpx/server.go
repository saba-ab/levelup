package httpx

import (
	"context"
	"net/http"
	"time"
)

// NewServer builds the listener with every timeout set (R10). An unset
// ReadHeaderTimeout is a slowloris invitation; unset Read/Write timeouts let
// one slow client hold a goroutine forever.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      requestTimeout + 5*time.Second, // handler deadline fires first
		IdleTimeout:       60 * time.Second,
	}
}

// Shutdown drains in-flight requests up to the given context's deadline.
func Shutdown(ctx context.Context, srv *http.Server) error {
	return srv.Shutdown(ctx)
}

func contextWithTimeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), requestTimeout)
}
