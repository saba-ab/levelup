package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/modules/activity/internal/testfakes"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

const (
	playerA2 = "0198d000-0000-7000-8000-0000000000a2"
	playerA3 = "0198d000-0000-7000-8000-0000000000a3"
)

func seed(h *harness, tenantID, playerID, eventType string, at time.Time) {
	a := domain.Activity{
		ID: id.NewID(), TenantID: tenantID, EventID: id.NewID(), EventType: eventType,
		PlayerID: playerID, OccurredAt: at, ReceivedAt: at, CreatedAt: at, UpdatedAt: at,
		Status: domain.StatusPending,
	}
	h.repo.Rows[a.ID] = a
}

func TestListLastSeenReturnsNewestPerPlayerInRequestOrder(t *testing.T) {
	h := newHarness(t, allPerms())
	seed(h, tenantA, playerA1, "login", start)
	seed(h, tenantA, playerA1, "purchase_completed", start.Add(time.Hour))
	seed(h, tenantA, playerA1, "login", start.Add(30*time.Minute))
	seed(h, tenantA, playerA2, "login", start.Add(2*time.Hour))
	seed(h, tenantB, playerA3, "login", start.Add(3*time.Hour)) // another tenant
	seed(h, tenantA, "", "login", start.Add(4*time.Hour))       // unresolved player

	got, err := h.svc.ListLastSeen(as(tenantA), []string{playerA2, playerA3, playerA1, playerA2})
	require.NoError(t, err)
	require.Equal(t, []domain.LastSeen{
		{PlayerID: playerA2, At: start.Add(2 * time.Hour), EventType: "login"},
		{PlayerID: playerA1, At: start.Add(time.Hour), EventType: "purchase_completed"},
	}, got, "deduplicated, request order, foreign tenant's player absent")
}

func TestListLastSeenValidationAndAuthz(t *testing.T) {
	h := newHarness(t, allPerms())

	_, err := h.svc.ListLastSeen(as(tenantA), nil)
	require.Equal(t, "invalid_player_ids", errs.CodeOf(err))
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	_, err = h.svc.ListLastSeen(as(tenantA), []string{playerA1, "nope"})
	require.Equal(t, "invalid_player_ids", errs.CodeOf(err))

	many := make([]string, contracts.MaxLastSeenIDs+1)
	for i := range many {
		many[i] = id.NewID()
	}
	_, err = h.svc.ListLastSeen(as(tenantA), many)
	require.ErrorIs(t, err, domain.ErrTooManyPlayerIDs)

	_, err = h.svc.ListLastSeen(as(tenantA), append(many[:contracts.MaxLastSeenIDs], many[0]))
	require.NoError(t, err, "duplicates do not count towards the cap")

	denied := newHarness(t, testfakes.AllowKeys{contracts.PermView.Key(): true})
	_, err = denied.svc.ListLastSeen(as(tenantA), []string{playerA1})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	_, err = h.svc.ListLastSeen(context.Background(), []string{playerA1})
	require.Error(t, err, "no tenant principal")
}

func TestReaderLastSeen(t *testing.T) {
	h := newHarness(t, allPerms())
	seed(h, tenantA, playerA1, "login", start)
	seed(h, tenantA, playerA1, "login", start.Add(time.Minute))
	seed(h, tenantB, playerA2, "login", start)

	var r contracts.Reader = h.svc
	got, err := r.LastSeen(context.Background(), tenantA, []string{playerA1, playerA2, "garbage", ""})
	require.NoError(t, err)
	require.Equal(t, map[string]time.Time{playerA1: start.Add(time.Minute)}, got)

	empty, err := r.LastSeen(context.Background(), tenantA, nil)
	require.NoError(t, err)
	require.Empty(t, empty)

	_, err = r.LastSeen(context.Background(), "", []string{playerA1})
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	var many []string
	for range contracts.MaxLastSeenIDs + 1 {
		many = append(many, id.NewID())
	}
	_, err = r.LastSeen(context.Background(), tenantA, many)
	require.ErrorIs(t, err, domain.ErrTooManyPlayerIDs)
}
