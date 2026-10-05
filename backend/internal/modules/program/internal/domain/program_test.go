package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

var t0 = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func mustProgram(t *testing.T, status Status) Program {
	t.Helper()
	p, err := NewProgram(NewProgramInput{TenantID: "t1", Name: "Loyalty Rewards"}, t0)
	require.NoError(t, err)
	p.Status = status
	return p
}

func TestNewProgram(t *testing.T) {
	long := make([]byte, 256)
	for i := range long {
		long[i] = 'a'
	}
	cases := []struct {
		name string
		in   NewProgramInput
		err  error
		slug string
	}{
		{"derives slug", NewProgramInput{TenantID: "t1", Name: "  Holiday Special 2026! "}, nil, "holiday-special-2026"},
		{"explicit slug", NewProgramInput{TenantID: "t1", Name: "x", Slug: "my-slug"}, nil, "my-slug"},
		{"no tenant", NewProgramInput{Name: "x"}, ErrNoTenant, ""},
		{"blank name", NewProgramInput{TenantID: "t1", Name: "   "}, ErrNameRequired, ""},
		{"long name", NewProgramInput{TenantID: "t1", Name: string(long)}, ErrNameTooLong, ""},
		{"bad slug", NewProgramInput{TenantID: "t1", Name: "x", Slug: "Bad Slug"}, ErrBadSlug, ""},
		{"underivable slug", NewProgramInput{TenantID: "t1", Name: "პროგრამა"}, ErrBadSlug, ""},
		{"window reversed", NewProgramInput{TenantID: "t1", Name: "x",
			StartsAt: ptr(t0.Add(time.Hour)), EndsAt: ptr(t0)}, ErrBadWindow, ""},
		{"window equal ok", NewProgramInput{TenantID: "t1", Name: "x", StartsAt: ptr(t0), EndsAt: ptr(t0)}, nil, "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewProgram(tc.in, t0)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, StatusDraft, p.Status, "a new program is always draft")
			require.Equal(t, tc.slug, p.Slug)
			require.NotNil(t, p.Settings)
			require.NotNil(t, p.Mechanics)
		})
	}
}

func TestStateMachine(t *testing.T) {
	type action struct {
		name string
		do   func(*Program, time.Time) error
		to   Status
	}
	activate := action{"activate", (*Program).Activate, StatusActive}
	pause := action{"pause", (*Program).Pause, StatusPaused}
	end := action{"end", (*Program).End, StatusEnded}

	cases := []struct {
		from Status
		act  action
		ok   bool
	}{
		{StatusDraft, activate, true},
		{StatusDraft, pause, false},
		{StatusDraft, end, false},
		{StatusActive, activate, false},
		{StatusActive, pause, true},
		{StatusActive, end, true},
		{StatusPaused, activate, true},
		{StatusPaused, pause, false},
		{StatusPaused, end, true},
		{StatusEnded, activate, false},
		{StatusEnded, pause, false},
		{StatusEnded, end, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"→"+tc.act.name, func(t *testing.T) {
			p := mustProgram(t, tc.from)
			err := tc.act.do(&p, t0.Add(time.Minute))
			if tc.ok {
				require.NoError(t, err)
				require.Equal(t, tc.act.to, p.Status)
				require.Equal(t, t0.Add(time.Minute), p.UpdatedAt)
				return
			}
			require.ErrorIs(t, err, ErrInvalidTransition)
			require.Equal(t, errs.Conflict, errs.KindOf(err))
			require.Equal(t, "invalid_status_transition", errs.CodeOf(err))
			require.Equal(t, tc.from, p.Status, "a refused transition leaves the status alone")
		})
	}
}

func TestActivateRequiresFutureEndsAt(t *testing.T) {
	p := mustProgram(t, StatusDraft)
	p.EndsAt = ptr(t0)
	require.ErrorIs(t, p.Activate(t0), ErrWindowElapsed, "ends_at == now is already over")
	require.Equal(t, StatusDraft, p.Status)

	p.EndsAt = ptr(t0.Add(time.Second))
	require.NoError(t, p.Activate(t0))
}

func TestAcceptsEnrollment(t *testing.T) {
	cases := []struct {
		name     string
		status   Status
		starts   *time.Time
		ends     *time.Time
		expected bool
	}{
		{"active open window", StatusActive, nil, nil, true},
		{"active inside", StatusActive, ptr(t0.Add(-time.Hour)), ptr(t0.Add(time.Hour)), true},
		{"active on bounds", StatusActive, ptr(t0), ptr(t0), true},
		{"active not started", StatusActive, ptr(t0.Add(time.Second)), nil, false},
		{"active over", StatusActive, nil, ptr(t0.Add(-time.Second)), false},
		{"draft", StatusDraft, nil, nil, false},
		{"paused", StatusPaused, nil, nil, false},
		{"ended", StatusEnded, nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := mustProgram(t, tc.status)
			p.StartsAt, p.EndsAt = tc.starts, tc.ends
			require.Equal(t, tc.expected, p.AcceptsEnrollment(t0))
		})
	}
}

func TestDueForAutoEnd(t *testing.T) {
	past, future := ptr(t0.Add(-time.Minute)), ptr(t0.Add(time.Minute))
	cases := []struct {
		status Status
		ends   *time.Time
		due    bool
	}{
		{StatusActive, past, true},
		{StatusPaused, past, true},
		{StatusActive, future, false},
		{StatusActive, nil, false},
		{StatusDraft, past, false},
		{StatusEnded, past, false},
	}
	for _, tc := range cases {
		p := mustProgram(t, tc.status)
		p.EndsAt = tc.ends
		require.Equal(t, tc.due, p.DueForAutoEnd(t0), "%s ends=%v", tc.status, tc.ends)
	}
}

func TestEditLeavesOmittedFieldsUntouched(t *testing.T) {
	p, err := NewProgram(NewProgramInput{
		TenantID: "t1", Name: "Loyalty", Description: ptr("desc"),
		StartsAt: ptr(t0), Settings: map[string]any{"welcome_points": float64(100)},
	}, t0)
	require.NoError(t, err)
	p.Status = StatusActive

	fields, err := p.Edit(Patch{Name: ptr("Loyalty 2")}, t0.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, []string{"name"}, fields)
	require.Equal(t, "Loyalty 2", p.Name)
	require.Equal(t, "desc", *p.Description, "omitted description must survive (Laravel PUT nulled it)")
	require.Equal(t, t0, *p.StartsAt)
	require.Equal(t, float64(100), p.Settings["welcome_points"])
	require.Equal(t, StatusActive, p.Status, "edit never touches status")
	require.Equal(t, t0.Add(time.Minute), p.UpdatedAt)
}

func TestEditClearsNullableFields(t *testing.T) {
	p, err := NewProgram(NewProgramInput{TenantID: "t1", Name: "x", Description: ptr("d"), StartsAt: ptr(t0)}, t0)
	require.NoError(t, err)

	fields, err := p.Edit(Patch{
		Description: Nullable[string]{Set: true},
		StartsAt:    Nullable[time.Time]{Set: true},
	}, t0)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"description", "starts_at"}, fields)
	require.Nil(t, p.Description)
	require.Nil(t, p.StartsAt)
}

func TestEditNoChangeReportsNothing(t *testing.T) {
	p := mustProgram(t, StatusDraft)
	before := p
	fields, err := p.Edit(Patch{Name: ptr(p.Name), Settings: map[string]any{}}, t0.Add(time.Hour))
	require.NoError(t, err)
	require.Empty(t, fields)
	require.Equal(t, before, p)
}

func TestEditRejectsInvalidAndLeavesProgramUnchanged(t *testing.T) {
	p := mustProgram(t, StatusDraft)
	p.StartsAt = ptr(t0)
	before := p

	_, err := p.Edit(Patch{Name: ptr("new"), EndsAt: Nullable[time.Time]{Set: true, Value: ptr(t0.Add(-time.Hour))}}, t0)
	require.ErrorIs(t, err, ErrBadWindow)
	require.Equal(t, before, p)

	_, err = p.Edit(Patch{Name: ptr("  ")}, t0)
	require.ErrorIs(t, err, ErrNameRequired)
	_, err = p.Edit(Patch{Slug: ptr("UPPER")}, t0)
	require.ErrorIs(t, err, ErrBadSlug)
	require.Equal(t, before, p)
}

func TestParseStatus(t *testing.T) {
	for _, s := range []string{"draft", "active", "paused", "ended"} {
		got, err := ParseStatus(s)
		require.NoError(t, err)
		require.Equal(t, Status(s), got)
	}
	_, err := ParseStatus("archived")
	require.ErrorIs(t, err, ErrBadStatus)
}
