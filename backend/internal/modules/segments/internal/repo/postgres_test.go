package repo

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/segments/internal/app"
	"levelup/internal/modules/segments/internal/domain"
	"levelup/internal/modules/segments/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

var t0 = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "segments", migrations.FS))
	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "segments")
	return NewPostgres(moduleDB), moduleDB
}

func inTx(t *testing.T, db *gorm.DB, fn func(tx *gorm.DB) error) {
	t.Helper()
	require.NoError(t, postgres.InTx(context.Background(), db, fn))
}

func mustSegment(t *testing.T, r *Postgres, db *gorm.DB, tenant, name string) domain.Segment {
	t.Helper()
	s, err := domain.NewSegment(tenant, "u1", domain.NewSegmentInput{
		Name: name,
		Conditions: map[string]any{"all": []any{
			map[string]any{"field": "attributes.country", "op": "in", "value": []any{"GE", "AM"}},
			map[string]any{"any": []any{map[string]any{"field": "level", "op": "gte", "value": 3.0}}},
		}},
	}, t0)
	require.NoError(t, err)
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateSegment(context.Background(), tx, s) })
	return s
}

func TestCreateLoadAndNameUniquePerTenant(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, other := id.NewID(), id.NewID()
	s := mustSegment(t, r, db, tenant, "VIPs")

	got, err := r.SegmentByID(ctx, tenant, s.ID)
	require.NoError(t, err)
	require.Equal(t, s.Conditions, got.Conditions, "conditions round-trip through jsonb")
	require.Equal(t, "u1", got.CreatedBy)

	_, err = r.SegmentByID(ctx, other, s.ID)
	require.ErrorIs(t, err, domain.ErrSegmentNotFound, "another tenant's segment is 404")
	_, err = r.SegmentByID(ctx, tenant, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrSegmentNotFound)

	dup, err := domain.NewSegment(tenant, "u1", domain.NewSegmentInput{Name: "vips", Conditions: s.Conditions.ToMap()}, t0)
	require.NoError(t, err)
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateSegment(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrNameTaken, "names are unique per tenant, case-insensitively")
	mustSegment(t, r, db, other, "VIPs") // fine in another tenant
}

func TestSaveDetectsVersionConflict(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	s := mustSegment(t, r, db, id.NewID(), "a")
	name := "b"
	_, err := s.Apply(domain.SegmentPatch{Name: &name}, t0)
	require.NoError(t, err)
	inTx(t, db, func(tx *gorm.DB) error { return r.SaveSegment(ctx, tx, s) })
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveSegment(ctx, tx, s) })
	require.ErrorIs(t, err, domain.ErrVersionConflict)
}

func TestRefreshLeaseIsExclusiveUntilExpiry(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	s := mustSegment(t, r, db, tenant, "a")
	run1, run2 := id.NewID(), id.NewID()

	_, ok, err := r.AcquireRefresh(ctx, tenant, s.ID, run1, t0, t0.Add(10*time.Minute))
	require.NoError(t, err)
	require.True(t, ok)
	_, ok, err = r.AcquireRefresh(ctx, tenant, s.ID, run2, t0.Add(time.Minute), t0.Add(11*time.Minute))
	require.NoError(t, err)
	require.False(t, ok, "a held lease refuses a second run")

	_, ok, err = r.AcquireRefresh(ctx, tenant, s.ID, run2, t0.Add(10*time.Minute), t0.Add(20*time.Minute))
	require.NoError(t, err)
	require.True(t, ok, "an expired lease is taken over")

	inTx(t, db, func(tx *gorm.DB) error {
		held, err := r.ExtendLease(ctx, tx, s.ID, run1, t0.Add(time.Hour))
		require.False(t, held, "the first run lost its lease")
		return err
	})
	_, _, err = r.AcquireRefresh(ctx, id.NewID(), s.ID, run1, t0, t0)
	require.ErrorIs(t, err, domain.ErrSegmentNotFound)
}

func TestApplyPageSweepAndFinish(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	s := mustSegment(t, r, db, tenant, "a")
	p1, p2, p3, gone := id.NewID(), id.NewID(), id.NewID(), id.NewID()
	run1 := id.NewID()

	var res app.PageResult
	inTx(t, db, func(tx *gorm.DB) (err error) {
		res, err = r.ApplyPage(ctx, tx, tenant, s.ID, run1, []string{p1, p2, gone}, []string{p1, p2, gone}, t0)
		return err
	})
	require.ElementsMatch(t, []string{p1, p2, gone}, res.Added)
	require.Empty(t, res.Removed)

	run2 := id.NewID()
	inTx(t, db, func(tx *gorm.DB) (err error) {
		// p1 still matches, p2 no longer does, p3 is new; gone is not listed.
		res, err = r.ApplyPage(ctx, tx, tenant, s.ID, run2, []string{p1, p2, p3}, []string{p1, p3}, t0.Add(time.Hour))
		return err
	})
	require.Equal(t, []string{p3}, res.Added)
	require.Equal(t, []string{p2}, res.Removed)

	inTx(t, db, func(tx *gorm.DB) (err error) {
		res, err = r.ApplyPage(ctx, tx, tenant, s.ID, run2, []string{p2}, nil, t0)
		return err
	})
	require.Empty(t, res.Added)
	require.Empty(t, res.Removed, "a page with no matches removes only existing members")

	var swept []string
	inTx(t, db, func(tx *gorm.DB) (err error) {
		swept, err = r.SweepStale(ctx, tx, s.ID, run2, 100)
		return err
	})
	require.Equal(t, []string{gone}, swept)

	_, ok, err := r.AcquireRefresh(ctx, tenant, s.ID, run2, t0, t0.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, r.FinishRefresh(ctx, s.ID, run2, t0.Add(2*time.Hour)))
	got, err := r.SegmentByID(ctx, tenant, s.ID)
	require.NoError(t, err)
	require.Equal(t, 2, got.MemberCount)
	require.NotNil(t, got.LastRefreshedAt)
	require.Nil(t, got.RefreshLeaseUntil)

	// p1 kept its original added_at; members page newest first.
	page, err := r.ListMembers(ctx, tenant, s.ID, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, page, 2)
	require.Equal(t, p3, page[0].PlayerID)
	require.Equal(t, p1, page[1].PlayerID)
	require.True(t, page[1].AddedAt.Equal(t0))
	next, err := r.ListMembers(ctx, tenant, s.ID, app.Page{Limit: 10, Before: page[0].AddedAt, BeforeID: page[0].PlayerID})
	require.NoError(t, err)
	require.Len(t, next, 1)
	require.Equal(t, p1, next[0].PlayerID)
	foreign, err := r.ListMembers(ctx, id.NewID(), s.ID, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, foreign)
}

func TestRemovePlayerSegmentsOfPlayersAndDelete(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	a, b := mustSegment(t, r, db, tenant, "a"), mustSegment(t, r, db, tenant, "b")
	p1, p2 := id.NewID(), id.NewID()
	run := id.NewID()
	for _, s := range []domain.Segment{a, b} {
		inTx(t, db, func(tx *gorm.DB) error {
			_, err := r.ApplyPage(ctx, tx, tenant, s.ID, run, []string{p1, p2}, []string{p1, p2}, t0)
			return err
		})
	}

	got, err := r.SegmentsOfPlayers(ctx, tenant, []string{p1, p2, "junk"})
	require.NoError(t, err)
	want := []string{a.ID, b.ID}
	sort.Strings(want)
	require.Equal(t, want, got[p1])

	var segs []string
	inTx(t, db, func(tx *gorm.DB) (err error) {
		segs, err = r.RemovePlayer(ctx, tx, tenant, p1)
		return err
	})
	require.ElementsMatch(t, []string{a.ID, b.ID}, segs)
	inTx(t, db, func(tx *gorm.DB) (err error) {
		segs, err = r.RemovePlayer(ctx, tx, tenant, p1)
		return err
	})
	require.Empty(t, segs, "idempotent")

	inTx(t, db, func(tx *gorm.DB) error { return r.SoftDeleteSegment(ctx, tx, tenant, a.ID, t0) })
	got, err = r.SegmentsOfPlayers(ctx, tenant, []string{p2})
	require.NoError(t, err)
	require.Equal(t, []string{b.ID}, got[p2], "deleted segments drop out")
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SoftDeleteSegment(ctx, tx, tenant, a.ID, t0) })
	require.ErrorIs(t, err, domain.ErrSegmentNotFound)

	refs, err := r.LiveSegmentsAfter(ctx, "", 100000)
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, ref := range refs {
		ids[ref.ID] = true
	}
	require.True(t, ids[b.ID])
	require.False(t, ids[a.ID])

	list, err := r.ListSegments(ctx, tenant, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, r.MarkRun(ctx, "segments.refresh", t0))
	require.NoError(t, r.MarkRun(ctx, "segments.refresh", t0.Add(time.Hour)))

	inTx(t, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) })
	_, err = r.SegmentByID(ctx, tenant, b.ID)
	require.ErrorIs(t, err, domain.ErrSegmentNotFound)
	got, err = r.SegmentsOfPlayers(ctx, tenant, []string{p2})
	require.NoError(t, err)
	require.Empty(t, got)
}
