package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"levelup/internal/config"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/platform/redis/redistest"
)

// TestActivityToEffectsFlow is the end-to-end proof of flow F2
// (docs/rewrite/00-target-architecture.md §5): an ingested activity reaches
// rules, rules issues job commands, points and progression apply them, the
// level reward pays out, and leaderboards project the credits — across six
// modules, through real Postgres and real outbox rows.
//
// RabbitMQ is replaced by pump: it reads unpublished outbox rows exactly as
// the dispatcher does and hands each one to every subscriber of its topic,
// or to the job named by a "job." topic, exactly as the worker would. The
// broker adds delivery, not semantics; redelivery is tested separately below
// by pumping every row a second time.
func TestActivityToEffectsFlow(t *testing.T) {
	ctx := context.Background()
	dsn := pgtest.DSN(t)
	redisAddr := redistest.Addr(t)

	t.Setenv("DB_WRITER_DSN", dsn)
	t.Setenv("DB_READER_DSN", dsn)
	t.Setenv("DB_MIGRATE_DSN", dsn)
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("REDIS_CORE_ADDR", redisAddr)
	t.Setenv("REDIS_CACHE_ADDR", redisAddr)
	t.Setenv("RABBIT_URL", "amqp://127.0.0.1:1/") // never dialled by the API
	t.Setenv("HTTP_RATE_LIMIT_PER_MINUTE", "100000")
	t.Setenv("MODULES_ENABLED", strings.Join(allModules, ","))
	cfg, err := config.Load()
	require.NoError(t, err)

	require.NoError(t, Migrate(ctx, cfg))
	a, err := New(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(a.Close)
	require.NoError(t, a.VerifyAuthz())
	api := &client{t: t, h: a.Router()}

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	pump := func(all bool) int { return pumpOutbox(t, ctx, a, pool, all) }

	// Tenant + owner.
	reg := api.do("POST", "/api/v1/auth/register", "", map[string]any{
		"name": "Flow Owner", "email": "flow@example.com", "password": "secret-password-1", "tenant_name": "Flow Co",
	}, http.StatusCreated)
	api.token = reg["access_token"].(string)

	// Configuration: a player, a two-level ladder paying 25 points at level 2,
	// a weekly "points earned" board, and a published rule.
	player := api.do("POST", "/api/v1/players", "", map[string]any{"external_id": "u-1", "display_name": "Nino"}, http.StatusCreated)
	playerID := player["id"].(string)
	api.do("POST", "/api/v1/levels", "", map[string]any{"level_number": 1, "name": "Rookie", "xp_required": 0}, http.StatusCreated)
	api.do("POST", "/api/v1/levels", "", map[string]any{"level_number": 2, "name": "Pro", "xp_required": 50, "points_reward": 25}, http.StatusCreated)
	board := api.do("POST", "/api/v1/leaderboards", "", map[string]any{
		"name": "Top Earners", "type": "points", "metric": "earned", "reset_frequency": "weekly",
	}, http.StatusCreated)
	rule := api.do("POST", "/api/v1/rules", "", map[string]any{
		"name": "Big purchase", "trigger_event": "purchase_completed",
		"conditions": []any{map[string]any{"source": "trigger", "field": "amount", "operator": "gte", "value": 100}},
		"actions": []any{
			map[string]any{"type": "credit_points", "amount": 50},
			map[string]any{"type": "grant_xp", "amount": 60},
		},
	}, http.StatusCreated)
	api.do("POST", "/api/v1/rules/"+rule["id"].(string)+"/publish", "", map[string]any{"version": 1}, http.StatusOK)
	pump(false) // settle configuration side effects (wallet opened on player.created.v1, ...)

	// The activity.
	accepted := api.do("POST", "/api/v1/activities", "", map[string]any{
		"event_id": "evt-1", "event_type": "purchase_completed", "player_external_id": "u-1",
		"properties": map[string]any{"amount": 120},
	}, http.StatusAccepted)
	activityID := accepted["activity_id"].(string)
	require.Positive(t, pump(false), "the activity must produce outbox work")

	assertState := func() {
		t.Helper()
		wallet := api.do("GET", "/api/v1/players/"+playerID+"/wallet", "", nil, http.StatusOK)
		require.EqualValues(t, 75, wallet["balance"], "50 from the rule + 25 level-2 reward")

		progress := api.do("GET", "/api/v1/players/"+playerID+"/progress", "", nil, http.StatusOK)
		require.EqualValues(t, 60, progress["total_xp"])
		require.EqualValues(t, 2, progress["current_level"].(map[string]any)["level_number"])

		act := api.do("GET", "/api/v1/activities/"+activityID, "", nil, http.StatusOK)
		require.Equal(t, "decided", act["status"])

		decisions := api.do("GET", "/api/v1/rules/decisions?activity_id="+activityID, "", nil, http.StatusOK)
		rows := decisions["data"].([]any)
		require.Len(t, rows, 1)
		decision := api.do("GET", "/api/v1/rules/decisions/"+rows[0].(map[string]any)["id"].(string), "", nil, http.StatusOK)
		effects := decision["effects"].([]any)
		require.Len(t, effects, 2)
		for _, e := range effects {
			require.Equal(t, "applied", e.(map[string]any)["status"], "every effect settles from its outcome fact")
		}

		entries := api.do("GET", "/api/v1/leaderboards/"+board["id"].(string)+"/entries", "", nil, http.StatusOK)
		ranked := entries["data"].([]any)
		require.Len(t, ranked, 1)
		require.EqualValues(t, 75, ranked[0].(map[string]any)["score"])
	}
	assertState()

	// At-least-once delivery: every row delivered again changes nothing.
	pump(true)
	pump(false)
	assertState()

	// A duplicate activity is accepted once.
	dup := api.do("POST", "/api/v1/activities", "", map[string]any{
		"event_id": "evt-1", "event_type": "purchase_completed", "player_external_id": "u-1",
		"properties": map[string]any{"amount": 120},
	}, http.StatusOK)
	require.Equal(t, true, dup["duplicate"])
	require.Zero(t, pump(false))
	assertState()
}

// pumpOutbox delivers outbox rows the way dispatcher + worker do and marks
// them published. With all=true it redelivers every row ever written.
// Returns how many rows it delivered, looping until the outbox is drained.
func pumpOutbox(t *testing.T, ctx context.Context, a *App, pool *pgxpool.Pool, all bool) int {
	t.Helper()
	delivered := 0
	for round := 0; round < 20; round++ {
		q := `SELECT id, event_id, topic, payload, occurred_at FROM outbox_svc.outbox_events WHERE published_at IS NULL ORDER BY id`
		if all && round == 0 {
			q = `SELECT id, event_id, topic, payload, occurred_at FROM outbox_svc.outbox_events ORDER BY id`
		}
		rows, err := pool.Query(ctx, q)
		require.NoError(t, err)
		type row struct {
			id      int64
			eventID string
			topic   string
			payload []byte
			at      time.Time
		}
		var batch []row
		for rows.Next() {
			var r row
			require.NoError(t, rows.Scan(&r.id, &r.eventID, &r.topic, &r.payload, &r.at))
			batch = append(batch, r)
		}
		rows.Close()
		require.NoError(t, rows.Err())
		if len(batch) == 0 {
			return delivered
		}
		for _, r := range batch {
			env := bus.Envelope{EventID: r.eventID, Topic: r.topic, OccurredAt: r.at, Payload: r.payload}
			require.NoErrorf(t, deliver(ctx, a, env), "delivering %s", r.topic)
			_, err := pool.Exec(ctx, `UPDATE outbox_svc.outbox_events SET published_at = now() WHERE id = $1`, r.id)
			require.NoError(t, err)
			delivered++
		}
	}
	t.Fatal("outbox did not drain in 20 rounds: an event loop between modules?")
	return delivered
}

func deliver(ctx context.Context, a *App, env bus.Envelope) error {
	if name, ok := strings.CutPrefix(env.Topic, "job."); ok {
		body, err := json.Marshal(env)
		if err != nil {
			return err
		}
		for _, m := range a.Modules {
			for _, j := range m.Jobs() {
				if j.Name == name {
					return j.Run(ctx, body)
				}
			}
		}
		return fmt.Errorf("no module runs job %q", name)
	}
	for _, m := range a.Modules {
		for _, s := range m.Subscriptions() {
			if s.Topic == env.Topic {
				if err := s.Handler(ctx, env); err != nil {
					return fmt.Errorf("%s handling %s: %w", m.Name(), env.Topic, err)
				}
			}
		}
	}
	return nil
}

type client struct {
	t     *testing.T
	h     http.Handler
	token string
}

func (c *client) do(method, path, idemKey string, body any, want int) map[string]any {
	c.t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(c.t, err)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	require.Equalf(c.t, want, rec.Code, "%s %s: %s", method, path, rec.Body.String())
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
	return out
}
