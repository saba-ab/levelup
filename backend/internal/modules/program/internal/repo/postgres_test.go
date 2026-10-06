package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/program/internal/app"
	"levelup/internal/modules/program/internal/domain"
	"levelup/internal/modules/program/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "program", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "program")
	return NewPostgres(moduleDB), moduleDB
}

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func mustCreate(t *testing.T, r *Postgres, db *gorm.DB, tenantID, name string, mutate func(*domain.Program)) domain.Program {
	t.Helper()
	p, err := domain.NewProgram(domain.NewProgramInput{
		TenantID: tenantID, Name: name, Settings: map[string]any{"welcome_points": float64(100)},
	}, now)
	require.NoError(t, err)
	if mutate != nil {
		mutate(&p)
	}
	require.NoError(t, postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		return r.Create(context.Background(), tx, p)
	}))
	return p
}

func TestRoundTripAndTenantScoping(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	p := mustCreate(t, r, db, tenant, "Round Trip", nil)

	got, err := r.ByID(ctx, tenant, p.ID)
	require.NoError(t, err)
	require.Equal(t, p.Slug, got.Slug)
	require.Equal(t, float64(100), got.Settings["welcome_points"])
	require.Equal(t, domain.StatusDraft, got.Status)

	_, err = r.ByID(ctx, id.NewID(), p.ID)
	require.ErrorIs(t, err, domain.ErrNotFound, "another tenant's row is NotFound")
}

func TestSlugUniquePerTenantAmongLiveRows(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	p := mustCreate(t, r, db, tenant, "Same Slug", nil)

	dup, err := domain.NewProgram(domain.NewProgramInput{TenantID: tenant, Name: "Same Slug"}, now)
	require.NoError(t, err)
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.Create(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrSlugTaken)

	mustCreate(t, r, db, id.NewID(), "Same Slug", nil) // other tenant: fine

	deleted := now
	p.DeletedAt = &deleted
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.Save(ctx, tx, p) }))
	mustCreate(t, r, db, tenant, "Same Slug", nil) // soft delete freed the slug
}

func TestSaveDetectsVersionConflict(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	p := mustCreate(t, r, db, tenant, "Versioned", nil)

	require.NoError(t, p.Activate(now))
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.Save(ctx, tx, p) }))

	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.Save(ctx, tx, p) })
	require.ErrorIs(t, err, domain.ErrVersionConflict, "a write from a stale snapshot is refused")

	got, err := r.ByID(ctx, tenant, p.ID)
	require.NoError(t, err)
	require.Equal(t, 1, got.Version)
	require.Equal(t, domain.StatusActive, got.Status)
}

func TestEnrollOnConflictAndRemovePlayerReturning(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	a := mustCreate(t, r, db, tenant, "A", func(p *domain.Program) { p.Status = domain.StatusActive })
	b := mustCreate(t, r, db, tenant, "B", nil)
	player := id.NewID()

	enroll := func(programID string) bool {
		var created bool
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			var err error
			created, err = r.Enroll(ctx, tx, domain.Enrollment{ProgramID: programID, TenantID: tenant, PlayerID: player, EnrolledAt: now})
			return err
		}))
		return created
	}
	require.True(t, enroll(a.ID))
	require.False(t, enroll(a.ID), "ON CONFLICT DO NOTHING")
	require.True(t, enroll(b.ID))

	e, ok, err := r.Enrollment(ctx, tenant, a.ID, player)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, now, e.EnrolledAt)

	ids, err := r.ActiveProgramIDsForPlayer(ctx, tenant, player)
	require.NoError(t, err)
	require.Equal(t, []string{a.ID}, ids, "draft program b is not active")

	page, err := r.ListEnrollments(ctx, tenant, a.ID, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, page, 1)

	var removed []domain.Enrollment
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		removed, err = r.RemovePlayer(ctx, tx, tenant, player)
		return err
	}))
	require.Len(t, removed, 2)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		removed, err = r.RemovePlayer(ctx, tx, tenant, player)
		return err
	}))
	require.Empty(t, removed, "redelivery removes nothing")
}

func TestDueForAutoEndAndPurge(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	past := now.Add(-time.Hour)
	due := mustCreate(t, r, db, tenant, "Due", func(p *domain.Program) { p.Status = domain.StatusPaused; p.EndsAt = &past })
	mustCreate(t, r, db, tenant, "Draft", func(p *domain.Program) { p.EndsAt = &past })

	got, err := r.DueForAutoEnd(ctx, now, 100)
	require.NoError(t, err)
	var found bool
	for _, p := range got {
		require.NotEqual(t, domain.StatusDraft, p.Status)
		found = found || p.ID == due.ID
	}
	require.True(t, found)

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		_, err := r.Enroll(ctx, tx, domain.Enrollment{ProgramID: due.ID, TenantID: tenant, PlayerID: id.NewID(), EnrolledAt: now})
		return err
	}))
	for range 2 {
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) }))
	}
	list, err := r.List(ctx, tenant, app.ListFilter{}, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestListSearchEscapesWildcardsAndCountsMembers(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	pct := mustCreate(t, r, db, tenant, "Summer 50% Off", nil)
	under := mustCreate(t, r, db, tenant, "Winter_Cup", nil)
	plain := mustCreate(t, r, db, tenant, "Autumn", nil)
	mustCreate(t, r, db, id.NewID(), "Summer elsewhere", nil)

	search := func(q string) []string {
		t.Helper()
		got, err := r.List(ctx, tenant, app.ListFilter{Search: q}, app.Page{Limit: 10})
		require.NoError(t, err)
		ids := make([]string, len(got))
		for i, p := range got {
			ids[i] = p.ID
		}
		return ids
	}
	require.Equal(t, []string{pct.ID}, search("summer"), "case-insensitive, tenant-scoped")
	require.Equal(t, []string{pct.ID}, search("%"), "% matches literally")
	require.Equal(t, []string{under.ID}, search("_"), "_ matches literally")
	require.Equal(t, []string{under.ID}, search("winter-cup"), "slug matches")
	require.Empty(t, search(`\`))
	require.Len(t, search(""), 3)

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		for _, p := range []string{id.NewID(), id.NewID()} {
			if _, err := r.Enroll(ctx, tx, domain.Enrollment{ProgramID: pct.ID, TenantID: tenant, PlayerID: p, EnrolledAt: now}); err != nil {
				return err
			}
		}
		_, err := r.Enroll(ctx, tx, domain.Enrollment{ProgramID: under.ID, TenantID: tenant, PlayerID: id.NewID(), EnrolledAt: now})
		return err
	}))
	counts, err := r.MemberCounts(ctx, tenant, []string{pct.ID, under.ID, plain.ID})
	require.NoError(t, err)
	require.Equal(t, map[string]int64{pct.ID: 2, under.ID: 1}, counts)

	foreign, err := r.MemberCounts(ctx, id.NewID(), []string{pct.ID})
	require.NoError(t, err)
	require.Empty(t, foreign)
}
