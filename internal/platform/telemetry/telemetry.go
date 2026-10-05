// Package telemetry wires the three observability pillars (PRD §7.11): zap
// for logs, OTel for traces, Prometheus for metrics — plus the admin handler
// (/metrics + pprof) that binds to the admin port, never the public listener.
package telemetry

import (
	"context"
	"net/http"
	"net/http/pprof"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Telemetry struct {
	Log      *zap.Logger
	Tracer   trace.Tracer
	Registry *prometheus.Registry
	TP       *sdktrace.TracerProvider
}

// New builds the stack. With no OTEL_EXPORTER_OTLP_ENDPOINT set, spans are
// still created with valid, propagatable trace IDs — they are just never
// exported. That keeps trace_id in every log line locally with zero infra.
func New(env string) (*Telemetry, func(), error) {
	log, err := newLogger(env)
	if err != nil {
		return nil, nil, err
	}

	res := sdkresource.NewSchemaless(
		semconv.ServiceName("myapp"),
		semconv.DeploymentEnvironment(env),
	)

	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		exp, err := otlptracehttp.New(context.Background())
		if err != nil {
			return nil, nil, err
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	}
	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	cleanup := func() {
		_ = tp.Shutdown(context.Background())
		_ = log.Sync()
	}

	return &Telemetry{
		Log:      log,
		Tracer:   tp.Tracer("myapp"),
		Registry: reg,
		TP:       tp,
	}, cleanup, nil
}

func newLogger(env string) (*zap.Logger, error) {
	if env == "prod" {
		return zap.NewProduction()
	}
	dev := zap.NewDevelopmentConfig()
	dev.DisableStacktrace = true
	return dev.Build()
}

// ModuleLogger pre-tags a logger so every record a module writes carries its
// name (PRD §6 Deps: "pre-tagged zap.String(\"module\", name)").
func ModuleLogger(l *zap.Logger, module string) *zap.Logger {
	return l.With(zap.String("module", module))
}

// AdminHandler serves the operational surface. It must only ever be mounted
// on the admin listener (PRD §7.11).
func (t *Telemetry) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(t.Registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}
