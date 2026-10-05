package httpx

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"myapp/internal/shared/errs"
	"myapp/internal/shared/id"
)

const requestTimeout = 30 * time.Second

// BaseMiddleware is the platform chain applied to every module router
// (PRD §6: "already scoped and has platform middleware applied").
// Order matters: request id → recover → deadline → metrics → access log.
func BaseMiddleware(log *zap.Logger, tracer trace.Tracer, reg *prometheus.Registry) []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		requestID,
		recoverer(log),
		deadline,
		metrics(reg),
		accessLog(log),
		traced(tracer),
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = id.NewID()
		}
		w.Header().Set("X-Request-Id", rid)
		next.ServeHTTP(w, r)
	})
}

func recoverer(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler { //nolint:errorlint // sentinel by contract
						panic(rec)
					}
					log.Error("panic recovered",
						zap.Any("panic", rec),
						zap.String("path", r.URL.Path),
						zap.Stack("stack"))
					Error(w, r, errs.New(errs.Internal, "internal error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// deadline guarantees no handler runs without a context deadline (R10). The
// server's WriteTimeout is the backstop; this is the one handlers observe.
func deadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := contextWithTimeout(r)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func metrics(reg *prometheus.Registry) func(http.Handler) http.Handler {
	dur := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "RED: duration by route pattern, method and status.",
		Buckets: []float64{.005, .01, .025, .05, .1, .2, .4, .8, 1.6, 3.2},
	}, []string{"route", "method", "status"})
	reg.MustRegister(dur)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w}
			start := time.Now()
			next.ServeHTTP(sw, r)

			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			dur.WithLabelValues(route, r.Method, strconv.Itoa(status)).
				Observe(time.Since(start).Seconds())
		})
	}
}

func accessLog(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w}
			start := time.Now()
			next.ServeHTTP(sw, r)

			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", sw.status),
				zap.Duration("elapsed", time.Since(start)),
				zap.String("request_id", w.Header().Get("X-Request-Id")),
			}
			if sc := trace.SpanContextFromContext(r.Context()); sc.HasTraceID() {
				fields = append(fields, zap.String("trace_id", sc.TraceID().String()))
			}
			log.Info("http", fields...)
		})
	}
}

// traced starts a server span per request so trace_id exists even without a
// collector. Route pattern is set after serving, when chi knows it.
func traced(tracer trace.Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := tracer.Start(r.Context(), r.Method+" "+r.URL.Path)
			defer span.End()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
