package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/player/contracts"
	"levelup/internal/modules/player/internal/app"
	"levelup/internal/modules/player/internal/domain"
	"levelup/internal/shared/errs"
)

func (f *fixture) named(t *testing.T, ctx context.Context, ext, name string) domain.Player {
	t.Helper()
	p, err := f.svc.Create(ctx, app.CreateCmd{ExternalID: ext, DisplayName: name})
	require.NoError(t, err)
	f.clock.Advance(time.Second)
	return p
}

// pageAll walks every page of q at the given page size.
func pageAll(t *testing.T, f *fixture, ctx context.Context, q app.ListQuery) []string {
	t.Helper()
	var out []string
	for range 20 {
		page, err := f.svc.List(ctx, q)
		require.NoError(t, err)
		out = append(out, ids(page.Players)...)
		if page.NextCursor == "" {
			return out
		}
		q.Cursor = page.NextCursor
	}
	t.Fatal("pagination did not terminate")
	return nil
}

func TestListSortsAreKeysetPaged(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	bravo := f.named(t, ctx, "e1", "bravo")
	alpha1 := f.named(t, ctx, "e2", "Alpha")
	charlie := f.named(t, ctx, "Charlie-ext", "") // falls back to external id
	alpha2 := f.named(t, ctx, "e4", "alpha")      // ties with alpha1 on the key
	delta := f.named(t, ctx, "e5", "delta")
	f.named(t, asTenant(tenantB), "e6", "aardvark") // another tenant

	firstAlpha, secondAlpha := alpha1.ID, alpha2.ID
	if secondAlpha < firstAlpha {
		firstAlpha, secondAlpha = secondAlpha, firstAlpha
	}

	require.Equal(t,
		[]string{firstAlpha, secondAlpha, bravo.ID, charlie.ID, delta.ID},
		pageAll(t, f, ctx, app.ListQuery{Limit: 2, Sort: contracts.SortDisplayName}))
	require.Equal(t,
		[]string{bravo.ID, alpha1.ID, charlie.ID, alpha2.ID, delta.ID},
		pageAll(t, f, ctx, app.ListQuery{Limit: 2, Sort: contracts.SortCreatedAsc}))
	require.Equal(t,
		[]string{delta.ID, alpha2.ID, charlie.ID, alpha1.ID, bravo.ID},
		pageAll(t, f, ctx, app.ListQuery{Limit: 2, Sort: contracts.SortCreatedDesc}))
	require.Equal(t,
		[]string{delta.ID, alpha2.ID, charlie.ID, alpha1.ID, bravo.ID},
		pageAll(t, f, ctx, app.ListQuery{Limit: 2}), "default is -created_at")
}

func TestListCreatedRange(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	p0 := f.named(t, ctx, "r0", "a") // t0
	p1 := f.named(t, ctx, "r1", "b") // t0+1s
	p2 := f.named(t, ctx, "r2", "c") // t0+2s

	from, to := p1.CreatedAt, p2.CreatedAt
	page, err := f.svc.List(ctx, app.ListQuery{CreatedFrom: &from, CreatedTo: &to})
	require.NoError(t, err)
	require.Equal(t, []string{p1.ID}, ids(page.Players), "from inclusive, to exclusive")

	page, err = f.svc.List(ctx, app.ListQuery{CreatedFrom: &from, Sort: contracts.SortCreatedAsc})
	require.NoError(t, err)
	require.Equal(t, []string{p1.ID, p2.ID}, ids(page.Players))

	page, err = f.svc.List(ctx, app.ListQuery{CreatedTo: &from})
	require.NoError(t, err)
	require.Equal(t, []string{p0.ID}, ids(page.Players))

	_, err = f.svc.List(ctx, app.ListQuery{CreatedFrom: &to, CreatedTo: &from})
	require.Equal(t, contracts.CodeInvalidCreatedRange, errs.CodeOf(err))
	_, err = f.svc.List(ctx, app.ListQuery{CreatedFrom: &from, CreatedTo: &from})
	require.Equal(t, contracts.CodeInvalidCreatedRange, errs.CodeOf(err), "empty range")
}

func TestListRejectsUnknownSortAndForeignCursor(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	for _, n := range []string{"a", "b", "c"} {
		f.named(t, ctx, n, n)
	}
	_, err := f.svc.List(ctx, app.ListQuery{Sort: "email"})
	require.Equal(t, contracts.CodeInvalidSort, errs.CodeOf(err))

	byName, err := f.svc.List(ctx, app.ListQuery{Limit: 1, Sort: contracts.SortDisplayName})
	require.NoError(t, err)
	require.NotEmpty(t, byName.NextCursor)
	_, err = f.svc.List(ctx, app.ListQuery{Limit: 1, Sort: contracts.SortCreatedAsc, Cursor: byName.NextCursor})
	require.Equal(t, errs.Invalid, errs.KindOf(err), "a cursor is bound to its sort")
	_, err = f.svc.List(ctx, app.ListQuery{Limit: 1, Cursor: byName.NextCursor})
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	byDefault, err := f.svc.List(ctx, app.ListQuery{Limit: 1})
	require.NoError(t, err)
	_, err = f.svc.List(ctx, app.ListQuery{Limit: 1, Sort: contracts.SortDisplayName, Cursor: byDefault.NextCursor})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}
