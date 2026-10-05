package integration

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/modkit"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/errs"
)

// fixtureModule is a test-only module: the smallest thing that exercises
// the async spine THROUGH THE PUBLIC MODULE CONTRACT — HTTP → service →
// outbox (same tx) → dispatcher → broker → its own subscriber. The template
// ships no business modules, so the R34/R39/R46 proofs run against this
// instead. It is shaped exactly like a real module (docs/examples.md): a
// versioned topic in its "contracts", a handler that publishes inside the
// transaction, and a subscriber that is idempotent by construction.
type fixtureModule struct {
	deps     modkit.Deps
	received chan bus.Envelope
}

const (
	fixtureTopic = "fixture.created.v1"
	fixtureGroup = "fixture"
)

type fixtureCreatedV1 struct {
	Marker string    `json:"marker"`
	At     time.Time `json:"at"`
}

// fixtureMigrations lands in schema fixture_svc like any module's embed.FS.
var fixtureMigrations fs.FS = fstest.MapFS{
	"0001_init.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
CREATE TABLE received (
    event_id   TEXT PRIMARY KEY,
    marker     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE received;
`)},
}

func newFixtureModule(d modkit.Deps) *fixtureModule {
	return &fixtureModule{deps: d, received: make(chan bus.Envelope, 10)}
}

func (m *fixtureModule) Name() string { return "fixture" }

func (m *fixtureModule) Migrations() fs.FS { return fixtureMigrations }

// RegisterHTTP mounts the one write path. It lives under /internal so the
// OpenAPI drift test ignores it, exactly as it ignores module-to-module
// surfaces.
func (m *fixtureModule) RegisterHTTP(r chi.Router) {
	r.Post("/internal/fixture", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Marker string `json:"marker"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Marker == "" {
			http.Error(w, "marker required", http.StatusUnprocessableEntity)
			return
		}
		// The fact and the event commit together (PRD §8). The module has
		// no table of its own to write here; the outbox row IS the fact.
		err := postgres.InTx(r.Context(), m.deps.DB, func(tx *gorm.DB) error {
			return m.deps.Outbox.Publish(r.Context(), tx, fixtureTopic, fixtureCreatedV1{
				Marker: req.Marker, At: m.deps.Clock.Now(),
			})
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
}

func (m *fixtureModule) Subscriptions() []bus.Subscription {
	return []bus.Subscription{{
		Topic:   fixtureTopic,
		Group:   fixtureGroup,
		Handler: m.onCreated,
	}}
}

// onCreated is idempotent by construction: the row is keyed by event_id and
// a redelivery is ON CONFLICT DO NOTHING (PRD §8).
func (m *fixtureModule) onCreated(ctx context.Context, e bus.Envelope) error {
	var ev fixtureCreatedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable "+fixtureTopic, err)
	}
	if err := m.deps.DB.WithContext(ctx).Exec(
		`INSERT INTO fixture_svc.received (event_id, marker) VALUES (?, ?) ON CONFLICT DO NOTHING`,
		e.EventID, ev.Marker).Error; err != nil {
		return errs.Wrap(errs.Internal, "record fixture event", err)
	}
	m.received <- e
	return nil
}

func (m *fixtureModule) Jobs() []jobs.Job { return nil }

func (m *fixtureModule) Health(ctx context.Context) error {
	sqlDB, err := m.deps.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (m *fixtureModule) Permissions() []authz.Permission { return nil }
