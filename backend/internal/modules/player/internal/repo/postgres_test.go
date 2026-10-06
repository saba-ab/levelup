package repo

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/player/contracts"
	"levelup/internal/modules/player/internal/app"
	"levelup/internal/modules/player/internal/domain"
	"levelup/internal/modules/player/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "player", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "player")
	return NewPostgres(moduleDB), moduleDB
}

var base = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func newPlayer(t *testing.T, tenantID, ext string, at time.Time) domain.Player {
	t.Helper()
	p, err := domain.NewPlayer(id.NewID(), tenantID, ext, domain.Profile{
		DisplayName: "Name " + ext,
		Email:       ext + "@example.com",
		Attributes:  map[string]any{"tier": "gold", "nested": map[string]any{"n": float64(1)}},
	}, "", at)
	require.NoError(t, err)
	return p
}

func create(t *testing.T, r *Postgres, db *gorm.DB, p domain.Player) error {
	t.Helper()
	return postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		return r.Create(context.Background(), tx, p)
	})
}

func TestCreateRoundTripsAndEnforcesLiveUniqueness(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()

	p := newPlayer(t, tenant, "ext-1", base)
	require.NoError(t, create(t, r, db, p))

	got, err := r.ByID(ctx, tenant, p.ID)
	require.NoError(t, err)
	require.Equal(t, p.ExternalID, got.ExternalID)
	require.Equal(t, p.Attributes, got.Attributes, "JSONB round trip")
	require.Equal(t, base, got.CreatedAt)

	dup := newPlayer(t, tenant, "ext-1", base)
	require.ErrorIs(t, create(t, r, db, dup), domain.ErrExternalIDTaken, "ON CONFLICT on the partial index")

	other := newPlayer(t, id.NewID(), "ext-1", base)
	require.NoError(t, create(t, r, db, other), "uniqueness is per tenant")

	_, err = r.ByID(ctx, id.NewID(), p.ID)
	require.ErrorIs(t, err, domain.ErrNotFound, "another tenant's row is not found")
}

func TestRecreateAfterSoftDelete(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	p := newPlayer(t, tenant, "phoenix", base)
	require.NoError(t, create(t, r, db, p))

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		locked, err := r.ByIDForUpdate(ctx, tx, tenant, p.ID)
		if err != nil {
			return err
		}
		locked.MarkDeleted(base.Add(time.Minute))
		return r.Save(ctx, tx, locked)
	}))

	_, err := r.ByExternalID(ctx, tenant, "phoenix")
	require.ErrorIs(t, err, domain.ErrNotFound)

	again := newPlayer(t, tenant, "phoenix", base.Add(time.Hour))
	require.NoError(t, create(t, r, db, again), "a deleted external id is reusable (Laravel 500)")
	got, err := r.ByExternalID(ctx, tenant, "phoenix")
	require.NoError(t, err)
	require.Equal(t, again.ID, got.ID)
}

func TestSaveDetectsVersionConflictAndKeepsOmittedFields(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	p := newPlayer(t, tenant, "v", base)
	require.NoError(t, create(t, r, db, p))

	_, err := p.ApplyPatch(domain.Patch{Email: new("")}, base.Add(time.Minute))
	require.NoError(t, err)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.Save(ctx, tx, p) }))

	got, err := r.ByID(ctx, tenant, p.ID)
	require.NoError(t, err)
	require.Empty(t, got.Email, "cleared to NULL")
	require.Equal(t, "Name v", got.DisplayName)
	require.Equal(t, 1, got.Version)

	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.Save(ctx, tx, p) })
	require.ErrorIs(t, err, domain.ErrVersionConflict, "a save from a stale snapshot is refused")
}

func TestListKeysetSearchAndFilter(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	var made []domain.Player
	for i := range 5 {
		p := newPlayer(t, tenant, fmt.Sprintf("Alpha_%d", i), base.Add(time.Duration(i)*time.Second))
		if i == 2 {
			p.Active = false
		}
		require.NoError(t, create(t, r, db, p))
		made = append(made, p)
	}
	require.NoError(t, create(t, r, db, newPlayer(t, id.NewID(), "Alpha_x", base)))

	page, err := r.List(ctx, tenant, app.ListFilter{Limit: 2})
	require.NoError(t, err)
	require.Equal(t, []string{made[4].ID, made[3].ID}, idsOf(page))

	page, err = r.List(ctx, tenant, app.ListFilter{Limit: 10, HasAfter: true, After: made[3].CreatedAt, AfterID: made[3].ID})
	require.NoError(t, err)
	require.Equal(t, []string{made[2].ID, made[1].ID, made[0].ID}, idsOf(page))

	inactive := false
	page, err = r.List(ctx, tenant, app.ListFilter{Limit: 10, Active: &inactive})
	require.NoError(t, err)
	require.Equal(t, []string{made[2].ID}, idsOf(page))

	page, err = r.List(ctx, tenant, app.ListFilter{Limit: 10, Search: "alpha_1"})
	require.NoError(t, err)
	require.Equal(t, []string{made[1].ID}, idsOf(page), "case-insensitive prefix; '_' is literal, not a wildcard")

	page, err = r.List(ctx, tenant, app.ListFilter{Limit: 10, Search: "%"})
	require.NoError(t, err)
	require.Empty(t, page, "'%' is escaped")
}

func TestBatchReadsExcludeDeletedAndForeign(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	a := newPlayer(t, tenant, "a", base)
	b := newPlayer(t, tenant, "b", base)
	foreign := newPlayer(t, id.NewID(), "a", base)
	for _, p := range []domain.Player{a, b, foreign} {
		require.NoError(t, create(t, r, db, p))
	}
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		b.MarkDeleted(base)
		return r.Save(ctx, tx, b)
	}))

	got, err := r.ByIDs(ctx, tenant, []string{a.ID, b.ID, foreign.ID})
	require.NoError(t, err)
	require.Equal(t, []string{a.ID}, idsOf(got))

	got, err = r.ByExternalIDs(ctx, tenant, []string{"a", "b"})
	require.NoError(t, err)
	require.Equal(t, []string{a.ID}, idsOf(got))
}

func TestPurgeTenantBatchIsIdempotent(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, other := id.NewID(), id.NewID()
	for i := range 3 {
		require.NoError(t, create(t, r, db, newPlayer(t, tenant, fmt.Sprintf("p%d", i), base)))
	}
	keep := newPlayer(t, other, "p0", base)
	require.NoError(t, create(t, r, db, keep))

	purge := func(limit int) []app.Ref {
		var out []app.Ref
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			var err error
			out, err = r.PurgeTenantBatch(ctx, tx, tenant, limit)
			return err
		}))
		return out
	}
	first := purge(2)
	require.Len(t, first, 2)
	require.NotEmpty(t, first[0].ExternalID, "RETURNING feeds cache eviction")
	require.Len(t, purge(2), 1)
	require.Empty(t, purge(2), "redelivery deletes nothing")

	_, err := r.ByID(ctx, other, keep.ID)
	require.NoError(t, err, "other tenants untouched")
}

func idsOf(ps []domain.Player) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func TestCreateIfAbsentToleratesEveryUniqueKey(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	insertIfAbsent := func(p domain.Player) bool {
		t.Helper()
		var ok bool
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			var err error
			ok, err = r.CreateIfAbsent(ctx, tx, p)
			if err != nil {
				return err
			}
			// The transaction must still be usable after a collision.
			return tx.Exec("SELECT 1").Error
		}))
		return ok
	}

	p := newPlayer(t, tenant, "auto-1", base)
	p.CreatedBy = ""
	require.True(t, insertIfAbsent(p))

	again := p
	require.False(t, insertIfAbsent(again), "same id: primary key collision is a no-op")

	rival := newPlayer(t, tenant, "auto-1", base)
	require.False(t, insertIfAbsent(rival), "live (tenant, external_id) collision is a no-op")

	got, err := r.ByID(ctx, tenant, p.ID)
	require.NoError(t, err)
	require.Empty(t, got.CreatedBy, "created_by stays NULL for the system")

	// Soft-delete: the same derived id still collides; a new id succeeds.
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		got.MarkDeleted(base.Add(time.Minute))
		return r.Save(ctx, tx, got)
	}))
	require.False(t, insertIfAbsent(p), "a deleted player is not resurrected by a redelivered id")
	require.True(t, insertIfAbsent(rival), "the external id is free again for a new player")
}

func TestListSortsKeysetAndCreatedRange(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()

	mk := func(ext, name string, at time.Time) domain.Player {
		p, err := domain.NewPlayer(id.NewID(), tenant, ext, domain.Profile{DisplayName: name}, "", at)
		require.NoError(t, err)
		require.NoError(t, create(t, r, db, p))
		return p
	}
	bravo := mk("e1", "bravo", base)
	alpha1 := mk("e2", "Alpha", base.Add(time.Second))
	charlie := mk("Charlie-ext", "", base.Add(2*time.Second))
	alpha2 := mk("e4", "alpha", base.Add(3*time.Second))
	delta := mk("e5", "delta", base.Add(4*time.Second))
	foreign, err := domain.NewPlayer(id.NewID(), id.NewID(), "zz", domain.Profile{DisplayName: "aaa"}, "", base)
	require.NoError(t, err)
	require.NoError(t, create(t, r, db, foreign))

	walk := func(f app.ListFilter) []string {
		t.Helper()
		var out []string
		f.Limit = 2
		for range 10 {
			page, err := r.List(ctx, tenant, f)
			require.NoError(t, err)
			out = append(out, idsOf(page)...)
			if len(page) < f.Limit {
				return out
			}
			last := page[len(page)-1]
			f.HasAfter, f.After, f.AfterKey, f.AfterID = true, last.CreatedAt, last.SortName(), last.ID
		}
		t.Fatal("did not terminate")
		return nil
	}

	firstAlpha, secondAlpha := alpha1.ID, alpha2.ID
	if secondAlpha < firstAlpha {
		firstAlpha, secondAlpha = secondAlpha, firstAlpha
	}
	require.Equal(t, []string{firstAlpha, secondAlpha, bravo.ID, charlie.ID, delta.ID},
		walk(app.ListFilter{Sort: contracts.SortDisplayName}), "lower(COALESCE(display_name, external_id)), id")
	require.Equal(t, []string{bravo.ID, alpha1.ID, charlie.ID, alpha2.ID, delta.ID},
		walk(app.ListFilter{Sort: contracts.SortCreatedAsc}))
	require.Equal(t, []string{delta.ID, alpha2.ID, charlie.ID, alpha1.ID, bravo.ID},
		walk(app.ListFilter{Sort: contracts.SortCreatedDesc}))

	from, to := base.Add(time.Second), base.Add(3*time.Second)
	page, err := r.List(ctx, tenant, app.ListFilter{Sort: contracts.SortCreatedAsc, Limit: 10, CreatedFrom: &from, CreatedTo: &to})
	require.NoError(t, err)
	require.Equal(t, []string{alpha1.ID, charlie.ID}, idsOf(page), "from inclusive, to exclusive")

	var idx int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM pg_indexes WHERE indexname = 'ix_players_tenant_sort_name'`).Scan(&idx).Error)
	require.EqualValues(t, 1, idx, "migration 0004 created the index")
}

func TestIDsAfterPagesLiveIDsInOrder(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	var live []string
	for _, ext := range []string{"p1", "p2", "p3", "p4", "gone"} {
		p := newPlayer(t, tenant, ext, base)
		require.NoError(t, create(t, r, db, p))
		if ext == "gone" {
			require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
				p.MarkDeleted(base)
				return r.Save(ctx, tx, p)
			}))
			continue
		}
		live = append(live, p.ID)
	}
	require.NoError(t, create(t, r, db, newPlayer(t, id.NewID(), "foreign", base)))
	slices.Sort(live)

	first, err := r.IDsAfter(ctx, tenant, "", 3)
	require.NoError(t, err)
	require.Equal(t, live[:3], first)

	rest, err := r.IDsAfter(ctx, tenant, first[2], 3)
	require.NoError(t, err)
	require.Equal(t, live[3:], rest, "a short page is the last one; deleted and foreign rows are absent")
}
