package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

var testLimits = Limits{MaxAge: 30 * 24 * time.Hour, MaxFutureSkew: 5 * time.Minute, MaxPayloadBytes: 64}

func validInput() NewInput {
	return NewInput{
		TenantID:         "0198d000-0000-7000-8000-000000000001",
		EventID:          "evt-1",
		EventType:        "purchase_completed",
		PlayerExternalID: "ext-1",
	}
}

func TestNewActivityInvariants(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*NewInput)
		want   error
	}{
		{"valid", func(*NewInput) {}, nil},
		{"no tenant", func(in *NewInput) { in.TenantID = "" }, ErrNoTenant},
		{"blank event id", func(in *NewInput) { in.EventID = "  " }, ErrEventIDRequired},
		{"event id too long", func(in *NewInput) { in.EventID = strings.Repeat("e", 129) }, ErrEventIDTooLong},
		{"event id at limit", func(in *NewInput) { in.EventID = strings.Repeat("e", 128) }, nil},
		{"event type upper case", func(in *NewInput) { in.EventType = "Purchase" }, ErrBadEventType},
		{"event type with space", func(in *NewInput) { in.EventType = "a b" }, ErrBadEventType},
		{"event type too long", func(in *NewInput) { in.EventType = strings.Repeat("a", 101) }, ErrBadEventType},
		{"event type punctuation ok", func(in *NewInput) { in.EventType = "shop.order:paid-v2_x" }, nil},
		{"no player", func(in *NewInput) { in.PlayerExternalID = "" }, ErrPlayerRequired},
		{"internal player id only", func(in *NewInput) { in.PlayerExternalID = ""; in.PlayerID = "p1" }, nil},
		{"external id too long", func(in *NewInput) { in.PlayerExternalID = strings.Repeat("x", 256) }, ErrPlayerIDTooLong},
		{"future beyond skew", func(in *NewInput) { in.OccurredAt = testNow.Add(6 * time.Minute) }, ErrOccurredInFuture},
		{"future within skew", func(in *NewInput) { in.OccurredAt = testNow.Add(4 * time.Minute) }, nil},
		{"too old", func(in *NewInput) { in.OccurredAt = testNow.Add(-31 * 24 * time.Hour) }, ErrOccurredTooOld},
		{"old but in window", func(in *NewInput) { in.OccurredAt = testNow.Add(-29 * 24 * time.Hour) }, nil},
		{"properties too large", func(in *NewInput) { in.Properties = map[string]any{"k": strings.Repeat("v", 80)} }, ErrPropertiesTooLarge},
		{"context too large", func(in *NewInput) { in.Context = map[string]any{"k": strings.Repeat("v", 80)} }, ErrContextTooLarge},
		{"unmarshalable properties", func(in *NewInput) { in.Properties = map[string]any{"f": func() {}} }, ErrPayloadNotJSON},
		{"negative depth", func(in *NewInput) { in.CausationDepth = -1 }, ErrBadCausationDepth},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mutate(&in)
			a, err := NewActivity(in, testNow, testLimits)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, a.ID)
			require.Equal(t, StatusPending, a.Status)
		})
	}
}

func TestNewActivityDefaultsAndNormalises(t *testing.T) {
	in := validInput()
	in.EventID = "  evt-1  "
	a, err := NewActivity(in, testNow, testLimits)
	require.NoError(t, err)
	require.Equal(t, "evt-1", a.EventID)
	require.Equal(t, testNow, a.OccurredAt, "occurred_at defaults to now")
	require.Equal(t, testNow, a.ReceivedAt)
	require.NotNil(t, a.Properties)
	require.NotNil(t, a.Context)

	tbilisi := time.FixedZone("GET", 4*3600)
	in.OccurredAt = time.Date(2026, 10, 5, 15, 0, 0, 123456789, tbilisi)
	a, err = NewActivity(in, testNow, testLimits)
	require.NoError(t, err)
	require.Equal(t, time.UTC, a.OccurredAt.Location())
	require.Equal(t, 123456000, a.OccurredAt.Nanosecond(), "truncated to Postgres precision")
}

func TestZeroLimitsDisableChecks(t *testing.T) {
	in := validInput()
	in.OccurredAt = testNow.Add(-365 * 24 * time.Hour)
	in.Properties = map[string]any{"k": strings.Repeat("v", 10_000)}
	_, err := NewActivity(in, testNow, Limits{})
	require.NoError(t, err)
}

func TestValidStatus(t *testing.T) {
	for _, s := range []string{StatusPending, StatusDecided, StatusRejected} {
		require.True(t, ValidStatus(s))
	}
	require.False(t, ValidStatus("done"))
	require.False(t, ValidStatus(""))
}
