package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/missions/contracts"
)

var t0 = time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC) // a Wednesday

func intp(v int) *int              { return &v }
func timep(t time.Time) *time.Time { return &t }
func validParams() NewMissionParams {
	return NewMissionParams{TenantID: "t1", Name: "Daily Login!", Type: contracts.TypeRepeating, Target: 3}
}

func TestNewMissionInvariants(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*NewMissionParams)
		want error
	}{
		{"valid", func(*NewMissionParams) {}, nil},
		{"no tenant", func(p *NewMissionParams) { p.TenantID = "" }, ErrNoTenant},
		{"no name", func(p *NewMissionParams) { p.Name = "  " }, ErrNameRequired},
		{"bad type", func(p *NewMissionParams) { p.Type = "monthly" }, ErrBadType},
		{"bad initial status", func(p *NewMissionParams) { p.Status = contracts.MissionPaused }, ErrBadInitialStatus},
		{"zero target", func(p *NewMissionParams) { p.Target = 0 }, ErrBadTarget},
		{"negative points", func(p *NewMissionParams) { p.PointsReward = -1 }, ErrNegativeReward},
		{"negative xp", func(p *NewMissionParams) { p.XPReward = -1 }, ErrNegativeReward},
		{"max zero", func(p *NewMissionParams) { p.MaxCompletionsPerPlayer = intp(0) }, ErrBadMaxCompletions},
		{"one_time with max 2", func(p *NewMissionParams) {
			p.Type = contracts.TypeOneTime
			p.MaxCompletionsPerPlayer = intp(2)
		}, ErrOneTimeOnce},
		{"bad slug", func(p *NewMissionParams) { p.Slug = "Bad Slug" }, ErrBadSlug},
		{"window inverted", func(p *NewMissionParams) {
			p.StartsAt = timep(t0)
			p.EndsAt = timep(t0.Add(-time.Hour))
		}, ErrBadWindow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validParams()
			tc.mod(&p)
			_, err := NewMission(p, t0)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestNewMissionDefaults(t *testing.T) {
	p := validParams()
	p.Type = contracts.TypeOneTime
	m, err := NewMission(p, t0)
	require.NoError(t, err)
	require.Equal(t, "daily-login", m.Slug)
	require.Equal(t, contracts.MissionDraft, m.Status)
	require.NotNil(t, m.MaxCompletionsPerPlayer)
	require.Equal(t, 1, *m.MaxCompletionsPerPlayer, "one_time ⇒ max 1")
	require.NotNil(t, m.Criteria)
}

func TestTransitions(t *testing.T) {
	cases := []struct {
		from, to string
		ok       bool
	}{
		{contracts.MissionDraft, contracts.MissionActive, true},
		{contracts.MissionDraft, contracts.MissionPaused, false},
		{contracts.MissionActive, contracts.MissionPaused, true},
		{contracts.MissionPaused, contracts.MissionActive, true},
		{contracts.MissionActive, contracts.MissionExpired, true},
		{contracts.MissionExpired, contracts.MissionActive, false},
		{contracts.MissionExpired, contracts.MissionArchived, true},
		{contracts.MissionArchived, contracts.MissionActive, false},
		{contracts.MissionActive, contracts.MissionActive, true},
	}
	for _, tc := range cases {
		t.Run(tc.from+"→"+tc.to, func(t *testing.T) {
			m := Mission{Status: tc.from}
			err := m.TransitionTo(tc.to, t0)
			if tc.ok {
				require.NoError(t, err)
				require.Equal(t, tc.to, m.Status)
			} else {
				require.ErrorIs(t, err, ErrInvalidTransition)
			}
		})
	}
}

func TestAvailableWindowInclusive(t *testing.T) {
	m := Mission{Status: contracts.MissionActive, StartsAt: timep(t0), EndsAt: timep(t0.Add(time.Hour))}
	require.False(t, m.Available(t0.Add(-time.Second)))
	require.True(t, m.Available(t0))
	require.True(t, m.Available(t0.Add(time.Hour)))
	require.False(t, m.Available(t0.Add(time.Hour+time.Second)))
	m.Status = contracts.MissionPaused
	require.False(t, m.Available(t0))
}

func TestPeriodFor(t *testing.T) {
	key, end := PeriodFor(contracts.TypeDaily, t0)
	require.Equal(t, "2026-10-07", key)
	require.Equal(t, time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC), *end)

	key, end = PeriodFor(contracts.TypeWeekly, t0)
	require.Equal(t, "2026-W41", key)
	require.Equal(t, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), *end, "next Monday 00:00 UTC")

	sunday := time.Date(2026, 10, 11, 23, 59, 0, 0, time.UTC)
	key, _ = PeriodFor(contracts.TypeWeekly, sunday)
	require.Equal(t, "2026-W41", key, "Sunday belongs to the ISO week that started Monday")

	key, end = PeriodFor(contracts.TypeRepeating, t0)
	require.Equal(t, PeriodAll, key)
	require.Nil(t, end)
}

func TestAddProgressCapsAndCompletesOnce(t *testing.T) {
	m := Mission{ID: "m", TenantID: "t", Type: contracts.TypeRepeating, Target: 5}
	a := StartAttempt(m, "p", t0)
	done, err := a.AddProgress(3, t0)
	require.NoError(t, err)
	require.False(t, done)
	done, err = a.AddProgress(10, t0)
	require.NoError(t, err)
	require.True(t, done)
	require.Equal(t, int64(5), a.Progress, "capped at target")
	require.Equal(t, contracts.AttemptCompleted, a.Status)

	done, err = a.AddProgress(1, t0)
	require.ErrorIs(t, err, ErrAttemptNotOpen, "a completed attempt never completes again")
	require.False(t, done)
	require.ErrorIs(t, a.Complete(t0), ErrAttemptNotOpen)

	_, err = (&Attempt{Status: contracts.AttemptInProgress, Target: 1}).AddProgress(0, t0)
	require.ErrorIs(t, err, ErrNonPositiveIncrease)
}

func TestCompleteRequiresTarget(t *testing.T) {
	a := Attempt{Status: contracts.AttemptInProgress, Progress: 2, Target: 3}
	require.ErrorIs(t, a.Complete(t0), ErrNotCompleted)
	a.Progress = 3
	require.NoError(t, a.Complete(t0))
	require.NotNil(t, a.CompletedAt)
}

func TestCanStart(t *testing.T) {
	daily := Mission{Type: contracts.TypeDaily}
	require.NoError(t, daily.CanStart(10, false))
	require.ErrorIs(t, daily.CanStart(0, true), ErrLimitReached)

	capped := Mission{Type: contracts.TypeRepeating, MaxCompletionsPerPlayer: intp(2)}
	require.NoError(t, capped.CanStart(1, false))
	require.ErrorIs(t, capped.CanStart(2, false), ErrLimitReached)

	unlimited := Mission{Type: contracts.TypeRepeating}
	require.NoError(t, unlimited.CanStart(1000, true))
}

func TestChangeTypeOnlyInDraft(t *testing.T) {
	m := Mission{Type: contracts.TypeDaily, Status: contracts.MissionActive}
	require.ErrorIs(t, m.ChangeType(contracts.TypeWeekly), ErrTypeImmutable)
	m.Status = contracts.MissionDraft
	require.NoError(t, m.ChangeType(contracts.TypeOneTime))
	require.Equal(t, 1, *m.MaxCompletionsPerPlayer)
}
