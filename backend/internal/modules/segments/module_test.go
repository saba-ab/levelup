package segments

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	identitycontracts "levelup/internal/modules/identity/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/segments/contracts"
	"levelup/internal/modules/segments/migrations"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/modkit"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
	"levelup/internal/shared/validate"
)

// ---- port fakes ----

type players struct{ all map[string]PlayerSnapshot }

func (p players) PlayersByIDs(_ context.Context, tenantID string, ids []string) (map[string]PlayerSnapshot, error) {
	out := map[string]PlayerSnapshot{}
	for _, id := range ids {
		if s, ok := p.all[id]; ok && s.TenantID == tenantID {
			out[id] = s
		}
	}
	return out, nil
}

func (p players) ListPlayerIDs(_ context.Context, tenantID, afterID string, limit int) ([]string, error) {
	var ids []string
	for id, s := range p.all {
		if s.TenantID == tenantID && id > afterID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

type levels map[string]int

func (l levels) LevelsByPlayerIDs(context.Context, string, []string) (map[string]int, error) {
	return l, nil
}

type noWallets struct{}

func (noWallets) WalletsByPlayerIDs(context.Context, string, []string) (map[string]WalletSnapshot, error) {
	return nil, nil
}

type noBadges struct{}

func (noBadges) EarnedBadges(context.Context, string, []string) (map[string][]string, error) {
	return nil, nil
}

type noActivity struct{}

func (noActivity) LastSeen(context.Context, string, []string) (map[string]time.Time, error) {
	return nil, nil
}

type outbox struct{ published []bus.Envelope }

func (o *outbox) Publish(ctx context.Context, _ *gorm.DB, topic string, payload any) error {
	e, err := bus.NewEnvelope(ctx, clock.System(), topic, payload)
	o.published = append(o.published, e)
	return err
}

// End to end over HTTP and the refresh job against Postgres: create,
// refresh, list players, preview, delete.
func TestSegmentJourney(t *testing.T) {
	dsn := pgtest.DSN(t)
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "segments", migrations.FS))
	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)

	tenant := id.NewID()
	ids := []string{id.NewID(), id.NewID(), id.NewID()}
	sort.Strings(ids)
	all := map[string]PlayerSnapshot{}
	for i, pid := range ids {
		all[pid] = PlayerSnapshot{ID: pid, TenantID: tenant, ExternalID: "ext" + pid, DisplayName: "P", Active: true,
			Attributes: map[string]any{"tier": float64(i)}, CreatedAt: time.Now().UTC()}
	}
	ob := &outbox{}
	m := New(modkit.Deps{
		DB: postgres.NewModuleDB(base, "segments"), Outbox: ob, Authz: authz.AllowAll{},
		Clock: clock.System(), Validate: validate.New(),
	}, Config{RefreshSchedule: "15 * * * *", PageSize: 2, RefreshLease: time.Minute, PreviewLimit: 1000},
		players{all: all}, levels{ids[2]: 7}, noWallets{}, noBadges{}, noActivity{})

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(authz.Into(req.Context(), authz.Principal{UserID: "u1", TenantID: tenant})))
		})
	})
	m.RegisterHTTP(r)
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, &buf))
		return w
	}

	conditions := map[string]any{"any": []any{
		map[string]any{"field": "attributes.tier", "op": "gte", "value": 1},
		map[string]any{"field": "level", "op": "gte", "value": 5},
	}}
	w := call(http.MethodPost, "/segments", map[string]any{"name": "High tier", "conditions": conditions})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var seg struct {
		ID          string         `json:"id"`
		Conditions  map[string]any `json:"conditions"`
		MemberCount int            `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &seg))
	require.Len(t, ob.published, 1)
	require.Equal(t, "job."+contracts.JobRefreshSegment, ob.published[0].Topic)

	w = call(http.MethodPost, "/segments", map[string]any{"name": "high TIER", "conditions": conditions})
	require.Equal(t, http.StatusConflict, w.Code)
	w = call(http.MethodPost, "/segments", map[string]any{"name": "bad", "conditions": map[string]any{"all": []any{}}})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)

	// Run the queued job exactly as the worker would.
	var job func(context.Context, []byte) error
	for _, j := range m.Jobs() {
		if j.Name == contracts.JobRefreshSegment {
			job = j.Run
		}
	}
	body, err := json.Marshal(ob.published[0])
	require.NoError(t, err)
	ob.published = nil
	require.NoError(t, job(ctx, body))
	require.Len(t, ob.published, 2, "two players joined")
	for _, e := range ob.published {
		require.Equal(t, contracts.TopicMembershipChanged, e.Topic)
	}

	w = call(http.MethodGet, "/segments/"+seg.ID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &seg))
	require.Equal(t, 2, seg.MemberCount)

	w = call(http.MethodGet, "/segments/"+seg.ID+"/players?limit=1", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var page struct {
		Data []struct {
			PlayerID   string `json:"player_id"`
			ExternalID string `json:"external_id"`
		} `json:"data"`
		NextCursor string `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Data, 1)
	require.NotEmpty(t, page.NextCursor)
	require.Equal(t, "ext"+page.Data[0].PlayerID, page.Data[0].ExternalID)

	w = call(http.MethodPost, "/segments/preview", map[string]any{"conditions": map[string]any{
		"all": []any{map[string]any{"field": "attributes.tier", "op": "eq", "value": 0}}}})
	require.Equal(t, http.StatusOK, w.Code)
	var prev struct {
		CountEstimate int  `json:"count_estimate"`
		Scanned       int  `json:"scanned"`
		Complete      bool `json:"complete"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prev))
	require.Equal(t, 1, prev.CountEstimate)
	require.Equal(t, 3, prev.Scanned)
	require.True(t, prev.Complete)

	w = call(http.MethodPost, "/segments/"+seg.ID+"/refresh", nil)
	require.Equal(t, http.StatusAccepted, w.Code)

	got, err := m.Reader().SegmentsOfPlayers(ctx, tenant, ids)
	require.NoError(t, err)
	require.Len(t, got, 2)

	// player.deleted.v1 drops the membership.
	subs := map[string]bus.Handler{}
	for _, s := range m.Subscriptions() {
		subs[s.Topic] = s.Handler
	}
	ev, err := bus.NewEnvelope(ctx, clock.System(), playercontracts.TopicPlayerDeleted,
		playercontracts.PlayerDeletedV1{TenantID: tenant, PlayerID: ids[2]})
	require.NoError(t, err)
	require.NoError(t, subs[playercontracts.TopicPlayerDeleted](ctx, ev))
	got, err = m.Reader().SegmentsOfPlayers(ctx, tenant, ids)
	require.NoError(t, err)
	require.Len(t, got, 1)

	w = call(http.MethodDelete, "/segments/"+seg.ID, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	w = call(http.MethodGet, "/segments/"+seg.ID, nil)
	require.Equal(t, http.StatusNotFound, w.Code)

	ev, err = bus.NewEnvelope(ctx, clock.System(), identitycontracts.TopicTenantDeleted, identitycontracts.TenantDeletedV1{TenantID: tenant})
	require.NoError(t, err)
	require.NoError(t, subs[identitycontracts.TopicTenantDeleted](ctx, ev))
	w = call(http.MethodGet, "/segments", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"data":[],"next_cursor":""}`, w.Body.String())
}
