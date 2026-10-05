package bus_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
)

func TestNewEnvelopeSetsIdentityAndTime(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC))

	env, err := bus.NewEnvelope(context.Background(), fake, "user.registered.v1",
		map[string]string{"user_id": "u-1"})
	require.NoError(t, err)

	require.NotEmpty(t, env.EventID)
	require.Equal(t, "user.registered.v1", env.Topic)
	require.Equal(t, fake.Now(), env.OccurredAt)
	require.JSONEq(t, `{"user_id":"u-1"}`, string(env.Payload))
}

func TestNewEnvelopeCarriesTraceContext(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	env, err := bus.NewEnvelope(ctx, clock.System(), "t.v1", struct{}{})
	require.NoError(t, err)
	require.Contains(t, env.TraceCtx, "traceparent",
		"W3C context must ride the envelope so the trace survives the broker (R39)")
	require.Contains(t, env.TraceCtx["traceparent"], span.SpanContext().TraceID().String())
}

func TestInProcessFanOutDeliversToEveryGroup(t *testing.T) {
	b := bus.NewInProcess()

	var walletGot, notifGot []string
	sub := func(group string, sink *[]string) bus.Subscription {
		return bus.Subscription{
			Topic: "user.registered.v1",
			Group: group,
			Handler: func(_ context.Context, e bus.Envelope) error {
				*sink = append(*sink, e.EventID)
				return nil
			},
		}
	}
	require.NoError(t, b.Subscribe(context.Background(), sub("wallet", &walletGot)))
	require.NoError(t, b.Subscribe(context.Background(), sub("notification", &notifGot)))

	env, err := bus.NewEnvelope(context.Background(), clock.System(), "user.registered.v1", nil)
	require.NoError(t, err)
	require.NoError(t, b.Publish(context.Background(), env))

	require.Equal(t, []string{env.EventID}, walletGot)
	require.Equal(t, []string{env.EventID}, notifGot)
}

func TestInProcessIgnoresOtherTopics(t *testing.T) {
	b := bus.NewInProcess()
	called := false
	require.NoError(t, b.Subscribe(context.Background(), bus.Subscription{
		Topic: "wallet.debited.v1", Group: "g",
		Handler: func(context.Context, bus.Envelope) error { called = true; return nil },
	}))

	env, _ := bus.NewEnvelope(context.Background(), clock.System(), "user.registered.v1", nil)
	require.NoError(t, b.Publish(context.Background(), env))
	require.False(t, called)
}

func TestInProcessSurfacesHandlerError(t *testing.T) {
	b := bus.NewInProcess()
	boom := errors.New("boom")
	require.NoError(t, b.Subscribe(context.Background(), bus.Subscription{
		Topic: "t.v1", Group: "g",
		Handler: func(context.Context, bus.Envelope) error { return boom },
	}))

	env, _ := bus.NewEnvelope(context.Background(), clock.System(), "t.v1", nil)
	require.ErrorIs(t, b.Publish(context.Background(), env), boom)
}
