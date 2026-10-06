package repo_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/missions/internal/app"
	"levelup/internal/modules/missions/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/id"
)

func TestAutoMissionsFiltersByTenantStatusAndEventType(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()

	match := seedMission(t, r, db, tenant, nil) // purchase_completed, active
	seedMission(t, r, db, tenant, func(p *domain.NewMissionParams) { p.Criteria = map[string]any{"event_type": "login"} })
	seedMission(t, r, db, tenant, func(p *domain.NewMissionParams) { p.Status = contracts.MissionDraft })
	seedMission(t, r, db, tenant, func(p *domain.NewMissionParams) { p.Criteria = map[string]any{} })
	seedMission(t, r, db, id.NewID(), nil) // another tenant
	deleted := seedMission(t, r, db, tenant, nil)
	now := time.Now().UTC()
	deleted.DeletedAt = &now
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveMission(ctx, tx, deleted) }))

	got, err := r.AutoMissions(ctx, tenant, "purchase_completed")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, match.ID, got[0].ID)
	require.Equal(t, "purchase_completed", got[0].Criteria["event_type"])

	got, err = r.AutoMissions(ctx, "not-a-uuid", "purchase_completed")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestAttemptStatsGroupsPerMission(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	m := seedMission(t, r, db, tenant, func(p *domain.NewMissionParams) { p.Target = 1 })
	other := seedMission(t, r, db, tenant, nil)
	empty := seedMission(t, r, db, tenant, nil)

	base := time.Now().UTC().Truncate(time.Second).Add(-10 * time.Hour)
	insert := func(missionID string, status string, startedAt time.Time, completedAfter time.Duration) {
		t.Helper()
		mm := m
		if missionID == other.ID {
			mm = other
		}
		a := domain.StartAttempt(mm, id.NewID(), startedAt)
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			ok, err := r.InsertAttempt(ctx, tx, a)
			require.True(t, ok)
			if err != nil || status == contracts.AttemptInProgress {
				return err
			}
			end := startedAt.Add(completedAfter)
			res := tx.Exec(`UPDATE missions_svc.mission_attempts SET status = ?, progress = target,
				completed_at = CASE WHEN ? = 'completed' THEN ?::timestamptz END WHERE id = ?`, status, status, end, a.ID)
			return res.Error
		}))
	}
	insert(m.ID, contracts.AttemptCompleted, base, 2*time.Hour)
	insert(m.ID, contracts.AttemptCompleted, base, 4*time.Hour)
	insert(m.ID, contracts.AttemptInProgress, base, 0)
	insert(m.ID, contracts.AttemptExpired, base, 0)
	insert(other.ID, contracts.AttemptInProgress, base, 0)

	got, err := r.AttemptStats(ctx, tenant, []string{m.ID, other.ID, empty.ID, "junk"})
	require.NoError(t, err)
	require.Len(t, got, 2, "missions without attempts are absent")
	st := got[m.ID]
	require.EqualValues(t, 4, st.Started)
	require.EqualValues(t, 1, st.InProgress)
	require.EqualValues(t, 2, st.Completed)
	require.NotNil(t, st.AvgHoursToComplete)
	require.InDelta(t, 3.0, *st.AvgHoursToComplete, 1e-6)
	require.EqualValues(t, 1, got[other.ID].Started)
	require.Nil(t, got[other.ID].AvgHoursToComplete)

	foreign, err := r.AttemptStats(ctx, id.NewID(), []string{m.ID})
	require.NoError(t, err)
	require.Empty(t, foreign, "another tenant sees nothing")
}

func props(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

func TestHandleActivityAgainstPostgres(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	byAmount := seedMission(t, r, db, tenant, func(p *domain.NewMissionParams) {
		p.Type = contracts.TypeOneTime
		p.Target = 100
		p.Criteria = map[string]any{
			"event_type": "purchase_completed",
			"where":      []any{map[string]any{"field": "currency", "operator": "eq", "value": "USD"}},
			"increment":  map[string]any{"by": "property", "field": "amount"},
		}
	})
	ob := &syncOutbox{}
	svc := app.NewService(r, onePlayer{player, tenant}, ob, authz.AllowAll{}, db, clock.System(), 100)

	ev := func(activityID, properties string) activitycontracts.ReceivedV1 {
		return activitycontracts.ReceivedV1{
			ActivityID: activityID, TenantID: tenant, EventID: activityID, EventType: "purchase_completed",
			PlayerExternalID: "ext-" + player, Properties: props(t, properties),
		}
	}
	first := ev(id.NewID(), `{"currency": "USD", "amount": 60}`)
	res, err := svc.HandleActivity(ctx, first)
	require.NoError(t, err)
	require.Equal(t, 1, res.Applied)

	// Redelivery: the ledger key exists, nothing changes.
	res, err = svc.HandleActivity(ctx, first)
	require.NoError(t, err)
	require.Zero(t, res.Applied)
	require.Equal(t, 1, ob.count(contracts.TopicProgressUpdated))

	_, err = svc.HandleActivity(ctx, ev(id.NewID(), `{"currency": "EUR", "amount": 60}`))
	require.NoError(t, err)

	res, err = svc.HandleActivity(ctx, ev(id.NewID(), `{"currency": "USD", "amount": 70}`))
	require.NoError(t, err)
	require.Equal(t, 1, res.Complete)
	require.Equal(t, 1, ob.count(contracts.TopicCompleted))

	// One-time mission done: a further match is a quiet rejection that the
	// transaction rolls back — no ledger row, no publish.
	quiet := ev(id.NewID(), `{"currency": "USD", "amount": 5}`)
	res, err = svc.HandleActivity(ctx, quiet)
	require.NoError(t, err)
	require.Zero(t, res.Applied)
	require.Equal(t, 0, ob.count(contracts.TopicProgressRejected))
	_, found, err := r.ProgressEventByKey(ctx, tenant, app.AutoKey(quiet.ActivityID, byAmount.ID, player))
	require.NoError(t, err)
	require.False(t, found, "a quiet rejection leaves no ledger row")

	var events int64
	require.NoError(t, db.Table("missions_svc.progress_events").Where("tenant_id = ?", tenant).Count(&events).Error)
	require.EqualValues(t, 2, events)

	stats, err := r.AttemptStats(ctx, tenant, []string{byAmount.ID})
	require.NoError(t, err)
	require.EqualValues(t, 1, stats[byAmount.ID].Completed)
}
