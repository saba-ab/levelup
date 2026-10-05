package idempotency

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"

	"go.uber.org/zap"

	"levelup/internal/platform/authz"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
)

const (
	headerKey      = "Idempotency-Key"
	maxBodyBytes   = 1 << 20 // hash + replay cap; larger bodies bypass storage
	maxStoredBody  = 1 << 20
)

// HashRequest fingerprints (method+path, body): the mismatch check compares
// exactly these bytes (R11).
func HashRequest(routeLine string, body []byte) []byte {
	sum := sha256.Sum256(append([]byte(routeLine+"\x00"), body...))
	return sum[:]
}

// Middleware applies to non-GET requests carrying an Idempotency-Key.
// Replays return the stored response byte-for-byte; a reused key with a
// different body is a 422 (R11).
func Middleware(store *Store, log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(headerKey)
			if key == "" || r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
			if err != nil || len(body) > maxBodyBytes {
				httpx.Error(w, r, errs.New(errs.Invalid, "unreadable or oversized body for idempotent request"))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			key = scopedKey(r, key)
			state, stored, err := store.Claim(r.Context(), key,
				HashRequest(r.Method+" "+r.URL.Path, body))
			if err != nil {
				// The store being down must not take the API down: proceed
				// WITHOUT the guarantee, loudly.
				log.Warn("idempotency store unavailable — serving without replay protection", zap.Error(err))
				next.ServeHTTP(w, r)
				return
			}

			switch state {
			case Replay:
				if stored.ContentType != "" {
					w.Header().Set("Content-Type", stored.ContentType)
				}
				w.Header().Set("Idempotent-Replay", "true")
				w.WriteHeader(stored.Status)
				_, _ = w.Write(stored.Body)
				return
			case Mismatch:
				httpx.Error(w, r, errs.New(errs.Invalid,
					"Idempotency-Key was already used with a different request body"))
				return
			case InFlight:
				httpx.Error(w, r, errs.New(errs.Conflict,
					"an identical request is in flight — retry shortly"))
				return
			}

			// Claimed: run the handler, record what it answered.
			rec := &recorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			if rec.status == 0 || rec.buf.Len() > maxStoredBody || !final(rec.status) {
				_ = store.Release(r.Context(), key)
				return
			}
			if err := store.Complete(r.Context(), key, Stored{
				Status:      rec.status,
				ContentType: rec.Header().Get("Content-Type"),
				Body:        rec.buf.Bytes(),
			}); err != nil {
				log.Warn("idempotency record failed", zap.Error(err))
			}
		})
	}
}

type recorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}

// scopedKey namespaces the client's key by who sent it. Keys are chosen by
// clients ("k1", an order number), so two tenants — or two users — will pick
// the same one; unscoped, the second caller would receive the first one's
// stored response (ADR-0015). Anonymous calls share one namespace per path.
func scopedKey(r *http.Request, key string) string {
	if p, ok := authz.From(r.Context()); ok {
		return "t:" + p.TenantID + "|u:" + p.UserID + "|" + key
	}
	return "anon|" + key
}

// final reports whether a response settles the request for good. Auth
// failures, rate limiting and server errors say nothing about the operation
// itself: storing them would replay the failure forever and make a retry —
// the whole point of the key — impossible.
func final(status int) bool {
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden,
		status == http.StatusTooManyRequests, status >= 500:
		return false
	default:
		return true
	}
}
