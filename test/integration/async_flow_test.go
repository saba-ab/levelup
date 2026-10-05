package integration

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"

	"myapp/internal/platform/authz"
	"myapp/internal/platform/bus"
	"myapp/internal/platform/clock"
	"myapp/internal/platform/httpx"
	"myapp/internal/platform/modkit"
	"myapp/internal/platform/outbox"
	outboxmigrations "myapp/internal/platform/outbox/migrations"
	"myapp/internal/platform/postgres"
	"myapp/internal/platform/postgres/pgtest"
	"myapp/internal/platform/rabbit"
	"myapp/internal/platform/rabbit/rabbittest"
	"myapp/internal/platform/redis"
	"myapp/internal/platform/redis/redistest"
	"myapp/internal/shared/id"
	"myapp/internal/shared/validate"
)

// pipeline wires the fixture module end to end THROUGH ITS PUBLIC SURFACE,
// exactly as the composition root wires a real one: HTTP → module → outbox
// (same tx) → dispatcher (confirms) → rabbit → the module's subscriber.
type pipeline struct {
	srv           *httptest.Server
	router        chi.Router
	received      chan bus.Envelope
	exporter      *tracetest.InMemoryExporter
	countReceived func(t *testing.T, marker string) int
}

func startPipeline(t *testing.T, amqpURL, mgmtURL string) *pipeline {
	t.Helper()
	ctx := context.Background()
	dsn := pgtest.DSN(t)

	require.NoError(t, postgres.Apply(ctx, dsn, "outbox", outboxmigrations.FS))
	require.NoError(t, postgres.Apply(ctx, dsn, "fixture", fixtureMigrations))

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 8)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)

	// One Postgres serves every test in this package, and a previous test's
	// dispatcher may have been cancelled between a broker confirm and its
	// mark-published commit — at-least-once by design, which leaves a row
	// the NEXT pipeline's dispatcher would republish into an assertion
	// window that expects silence. Start from nothing.
	_, err = db.Writer().Exec(ctx,
		`TRUNCATE outbox_svc.outbox_events, outbox_svc.dead_letters, fixture_svc.received`)
	require.NoError(t, err)

	cache := redis.NewCache(redistest.Addr(t))
	store := outbox.NewPostgres(clock.System())
	val := validate.New()

	// deps mirrors Platform.DepsFor: schema-pinned GORM session,
	// prefix-bound cache, everything a module may touch.
	deps := func(module string) modkit.Deps {
		return modkit.Deps{
			DB:       postgres.NewModuleDB(base, module),
			SQL:      db,
			Cache:    redis.NewModuleCache(cache, module, time.Minute, zap.NewNop()),
			Redis:    redis.NewCore(redistest.Addr(t)),
			Bus:      bus.NewInProcess(),
			Outbox:   store,
			Clock:    clock.System(),
			Log:      zap.NewNop(),
			Validate: val,
			Authz:    authz.AllowAll{},
			Metrics:  prometheus.NewRegistry(),
			Tracer:   tp.Tracer("test"),
		}
	}

	mod := newFixtureModule(deps("fixture"))

	conn, err := rabbit.Dial(amqpURL, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	topo := rabbit.NewTopology(conn, rabbit.MgmtConfig{URL: mgmtURL, User: "guest", Password: "guest"}, zap.NewNop())
	require.NoError(t, topo.DeclareCore(ctx))

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	rb := bus.NewRabbit(conn, zap.NewNop())
	for _, sub := range mod.Subscriptions() {
		require.NoError(t, topo.DeclareEventQueue(ctx, sub.Group, sub.Topic))
		require.NoError(t, rb.Subscribe(runCtx, sub))
	}

	go func() { _ = outbox.NewDispatcher(db.Writer(), conn, zap.NewNop(), nil).Run(runCtx) }()

	r := chi.NewRouter()
	r.Use(httpx.BaseMiddleware(zap.NewNop(), tp.Tracer("http"), prometheus.NewRegistry())...)
	r.Route("/api/v1", func(api chi.Router) {
		mod.RegisterHTTP(api)
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	countReceived := func(t *testing.T, marker string) int {
		t.Helper()
		var n int
		require.NoError(t, db.Writer().QueryRow(context.Background(),
			`SELECT count(*) FROM fixture_svc.received WHERE marker = $1`, marker).Scan(&n))
		return n
	}

	return &pipeline{srv: srv, router: r, received: mod.received, exporter: exporter, countReceived: countReceived}
}

func createFixture(t *testing.T, p *pipeline, marker string) int {
	t.Helper()
	body := strings.NewReader(`{"marker":"` + marker + `"}`)
	resp, err := p.srv.Client().Post(p.srv.URL+"/api/v1/internal/fixture", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

// R39: one trace ID spans HTTP → outbox.publish → outbox.dispatch → consume.
func TestTraceSurvivesTheBroker(t *testing.T) {
	eps := rabbittest.Get(t)
	p := startPipeline(t, eps.AMQPURL, eps.MgmtURL)

	require.Equal(t, 201, createFixture(t, p, "flow-"+id.NewID()))

	select {
	case <-p.received:
	case <-time.After(20 * time.Second):
		t.Fatal("event never reached the consumer")
	}

	want := []string{
		"POST /api/v1/internal/fixture",
		"outbox.publish " + fixtureTopic,
		"outbox.dispatch " + fixtureTopic,
		"consume " + fixtureTopic,
	}
	require.Eventually(t, func() bool {
		byName := map[string]string{}
		for _, s := range p.exporter.GetSpans() {
			byName[s.Name] = s.SpanContext.TraceID().String()
		}
		var traceID string
		for _, n := range want {
			id, ok := byName[n]
			if !ok {
				return false
			}
			if traceID == "" {
				traceID = id
			}
			if id != traceID {
				t.Fatalf("span %q has trace %s, want %s — the trace broke at the broker (R39)", n, id, traceID)
			}
		}
		return true
	}, 20*time.Second, 200*time.Millisecond, "expected spans %v", want)
}

// R46/R34 chaos: stop the broker entirely, perform a state change, restart —
// the reaction happens exactly once, nothing lost.
func TestBrokerOutageLosesNothing(t *testing.T) {
	eps, stopBroker, startBroker, terminate := rabbittest.Fresh(t)
	defer terminate()

	p := startPipeline(t, eps.AMQPURL, eps.MgmtURL)

	stopBroker() // total broker outage

	// Unique per run so the exactly-once count below survives -count=N
	// against the package-shared database.
	marker := "chaos-" + id.NewID()

	// The user-facing request must still succeed: the fact commits with the
	// outbox row; the broker's availability is not the API's availability.
	require.Equal(t, 201, createFixture(t, p, marker),
		"state change must succeed with the broker down (PRD §7.9.2)")

	select {
	case e := <-p.received:
		t.Fatalf("nothing can be delivered while the broker is down; got event %s topic %s occurred %s",
			e.EventID, e.Topic, e.OccurredAt)
	case <-time.After(2 * time.Second):
	}

	startBroker() // recovery

	select {
	case <-p.received:
	case <-time.After(60 * time.Second):
		t.Fatal("outbox did not drain after broker recovery (R34)")
	}

	// Exactly once: give duplicates a moment to show up, then assert one row.
	time.Sleep(2 * time.Second)
	require.Equal(t, 1, p.countReceived(t, marker),
		"the reaction must happen exactly once across the outage (R46)")
}
