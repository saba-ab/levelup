package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/shared/errs"
)

func TestCreateLevelAndList(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	ctx := asUser("t1")

	for _, spec := range []domain.LevelSpec{
		{Number: 2, XPRequired: 100, PointsReward: 50, Active: true},
		{Number: 1, XPRequired: 0, Active: true},
		{Number: 3, XPRequired: 300, Active: false},
	} {
		_, err := h.svc.CreateLevel(ctx, spec)
		require.NoError(t, err)
	}
	h.seedLevel("t2", "foreign", 1, 0, 0, "")

	all, err := h.svc.ListLevels(ctx, false)
	require.NoError(t, err)
	require.Len(t, all, 3, "other tenants' levels are invisible")
	require.Equal(t, []int{1, 2, 3}, []int{all[0].Number, all[1].Number, all[2].Number})
	require.Equal(t, "Level 2", all[1].Name)

	active, err := h.svc.ListLevels(ctx, true)
	require.NoError(t, err)
	require.Len(t, active, 2)
}

func TestCreateLevelEnforcesLadder(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	ctx := asUser("t1")
	_, err := h.svc.CreateLevel(ctx, domain.LevelSpec{Number: 2, XPRequired: 100, Active: true})
	require.NoError(t, err)

	_, err = h.svc.CreateLevel(ctx, domain.LevelSpec{Number: 2, XPRequired: 500})
	require.Equal(t, errs.AlreadyExists, errs.KindOf(err))
	require.Equal(t, "level_number_taken", errs.CodeOf(err))

	_, err = h.svc.CreateLevel(ctx, domain.LevelSpec{Number: 3, XPRequired: 100})
	require.Equal(t, "xp_required_not_increasing", errs.CodeOf(err))

	_, err = h.svc.CreateLevel(ctx, domain.LevelSpec{Number: 1, XPRequired: 150})
	require.Equal(t, "xp_required_not_increasing", errs.CodeOf(err))

	_, err = h.svc.CreateLevel(ctx, domain.LevelSpec{Number: 1, XPRequired: 0, PointsReward: -1})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	require.Len(t, h.repo.levels, 1)
}

func TestCreateLevelNeedsAdminPermission(t *testing.T) {
	h := newHarness(t, memberKeys, Options{})
	_, err := h.svc.CreateLevel(asUser("t1"), domain.LevelSpec{Number: 1})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Empty(t, h.repo.levels)

	_, err = h.svc.ListLevels(asUser(""), false)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestUpdateLevel(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	h.seedLevel("t1", "l1", 1, 0, 0, "")
	h.seedLevel("t1", "l2", 2, 100, 0, "badge-x")
	h.seedLevel("t1", "l3", 3, 300, 0, "")
	h.clock.Advance(time.Hour)
	ctx := asUser("t1")

	clear := ""
	xp := int64(200)
	l, err := h.svc.UpdateLevel(ctx, "l2", domain.LevelPatch{XPRequired: &xp, BadgeRewardID: &clear})
	require.NoError(t, err)
	require.Equal(t, int64(200), l.XPRequired)
	require.Equal(t, "", l.BadgeRewardID, "null clears the reward (B20)")
	require.Equal(t, "l2", l.Name, "omitted fields stay")
	require.Equal(t, testNow.Add(time.Hour), l.UpdatedAt)

	tooHigh := int64(300)
	_, err = h.svc.UpdateLevel(ctx, "l2", domain.LevelPatch{XPRequired: &tooHigh})
	require.Equal(t, "xp_required_not_increasing", errs.CodeOf(err))

	taken := 3
	_, err = h.svc.UpdateLevel(ctx, "l2", domain.LevelPatch{Number: &taken})
	require.Equal(t, "level_number_taken", errs.CodeOf(err))
	require.Equal(t, int64(200), h.repo.levels["l2"].XPRequired)
}

func TestLevelCrossTenantIsNotFound(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	h.seedLevel("t2", "foreign", 1, 0, 0, "")
	ctx := asUser("t1")

	_, err := h.svc.GetLevel(ctx, "foreign")
	require.ErrorIs(t, err, domain.ErrLevelNotFound)

	name := "mine now"
	_, err = h.svc.UpdateLevel(ctx, "foreign", domain.LevelPatch{Name: &name})
	require.ErrorIs(t, err, domain.ErrLevelNotFound)

	err = h.svc.DeleteLevel(ctx, "foreign")
	require.ErrorIs(t, err, domain.ErrLevelNotFound)
	require.Equal(t, "foreign", h.repo.levels["foreign"].Name)
	require.Empty(t, h.repo.deleted)
}

func TestDeleteLevel(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	h.seedLevel("t1", "l1", 1, 0, 0, "")
	ctx := asUser("t1")

	require.NoError(t, h.svc.DeleteLevel(ctx, "l1"))
	_, err := h.svc.GetLevel(ctx, "l1")
	require.ErrorIs(t, err, domain.ErrLevelNotFound)

	// Its number is free again (B19).
	_, err = h.svc.CreateLevel(ctx, domain.LevelSpec{Number: 1, Active: true})
	require.NoError(t, err)

	denied := newHarness(t, memberKeys, Options{})
	denied.seedLevel("t1", "l1", 1, 0, 0, "")
	err = denied.svc.DeleteLevel(ctx, "l1")
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Empty(t, denied.repo.deleted)
}

func TestGetLevelRequiresView(t *testing.T) {
	h := newHarness(t, allowKeys{}, Options{})
	h.seedLevel("t1", "l1", 1, 0, 0, "")
	_, err := h.svc.GetLevel(context.Background(), "l1")
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
	_, err = h.svc.GetLevel(asUser("t1"), "l1")
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}
