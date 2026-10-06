package app

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

func TestListSearchesNameAndSlugCaseInsensitively(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	summer := h.create(t, ctx, "Summer Cup")
	h.create(t, ctx, "Winter League")
	h.create(t, ctxFor(tenantB), "Summer Foreign")

	got, _, err := h.svc.List(ctx, "", "  SUMMER ", "", 0)
	require.NoError(t, err)
	require.Equal(t, []string{summer.ID}, programIDs(got), "trimmed, case-insensitive, tenant-scoped")

	got, _, err = h.svc.List(ctx, "", "summer-c", "", 0)
	require.NoError(t, err)
	require.Equal(t, []string{summer.ID}, programIDs(got), "matches the slug too")

	got, _, err = h.svc.List(ctx, "active", "summer", "", 0)
	require.NoError(t, err)
	require.Empty(t, got, "search combines with the status filter")

	_, _, err = h.svc.List(ctx, "", strings.Repeat("x", MaxSearchLength+1), "", 0)
	requireCode(t, err, errs.Invalid, "invalid_search")
}

func TestMemberCountsIsOneQueryAndZeroFills(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	a := h.active(t, ctx, "a")
	b := h.active(t, ctx, "b")
	_, _, err := h.svc.Enroll(ctx, a.ID, player1)
	require.NoError(t, err)

	counts, err := h.svc.MemberCounts(ctx, []string{a.ID, b.ID})
	require.NoError(t, err)
	require.Equal(t, map[string]int64{a.ID: 1, b.ID: 0}, counts)
	require.Equal(t, 1, h.repo.countCalls, "one grouped query per page, never one per program")

	foreign, err := h.svc.MemberCounts(ctxFor(tenantB), []string{a.ID})
	require.NoError(t, err)
	require.Equal(t, map[string]int64{a.ID: 0}, foreign, "another tenant's program counts nothing")

	_, err = h.svc.MemberCounts(authz.Into(context.Background(), authz.Principal{UserID: "platform"}), []string{a.ID})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}
