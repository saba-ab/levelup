package telemetry_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"myapp/internal/platform/clock"
	"myapp/internal/platform/telemetry"
)

func TestFakeClock(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	fake := clock.NewFake(start)
	require.Equal(t, start, fake.Now())
	fake.Advance(time.Hour)
	require.Equal(t, start.Add(time.Hour), fake.Now())
}

func TestNewProvidesWorkingComponents(t *testing.T) {
	tel, cleanup, err := telemetry.New("test")
	require.NoError(t, err)
	defer cleanup()

	require.NotNil(t, tel.Log)
	require.NotNil(t, tel.Registry)

	// Tracer must mint real, propagatable trace IDs even with no exporter.
	ctx, span := tel.Tracer.Start(t.Context(), "probe")
	defer span.End()
	require.True(t, span.SpanContext().TraceID().IsValid())
	_ = ctx
}

func TestAdminHandlerServesMetricsAndPprof(t *testing.T) {
	tel, cleanup, err := telemetry.New("test")
	require.NoError(t, err)
	defer cleanup()

	srv := httptest.NewServer(tel.AdminHandler())
	defer srv.Close()

	for _, path := range []string{"/metrics", "/debug/pprof/"} {
		resp, err := srv.Client().Get(srv.URL + path)
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode, path)
		resp.Body.Close()
	}
}

func TestModuleLoggerTagsModule(t *testing.T) {
	tel, cleanup, err := telemetry.New("test")
	require.NoError(t, err)
	defer cleanup()

	// Just proves it does not panic and returns a distinct logger.
	ml := telemetry.ModuleLogger(tel.Log, "wallet")
	require.NotNil(t, ml)
	ml.Info("probe")
}
