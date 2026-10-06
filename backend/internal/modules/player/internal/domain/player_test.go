package domain

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func TestNewPlayerInvariants(t *testing.T) {
	cases := []struct {
		name     string
		tenant   string
		ext      string
		prof     Profile
		wantErr  error
		wantExt  string
		wantMail string
	}{
		{name: "minimal", tenant: "t1", ext: "ext-1", wantExt: "ext-1"},
		{name: "trims external id", tenant: "t1", ext: "  ext-1 ", wantExt: "ext-1"},
		{name: "lower-cases email", tenant: "t1", ext: "e", prof: Profile{Email: " Bob@Example.COM "}, wantExt: "e", wantMail: "bob@example.com"},
		{name: "no tenant", tenant: "", ext: "e", wantErr: ErrNoTenant},
		{name: "blank external id", tenant: "t1", ext: "   ", wantErr: ErrNoExternalID},
		{name: "external id too long", tenant: "t1", ext: strings.Repeat("x", 256), wantErr: ErrExternalIDTooLong},
		{name: "external id at limit", tenant: "t1", ext: strings.Repeat("x", 255), wantExt: strings.Repeat("x", 255)},
		{name: "bad email", tenant: "t1", ext: "e", prof: Profile{Email: "not-an-email"}, wantErr: ErrEmailInvalid},
		{name: "email with display part", tenant: "t1", ext: "e", prof: Profile{Email: "Bob <bob@x.io>"}, wantErr: ErrEmailInvalid},
		{name: "display name too long", tenant: "t1", ext: "e", prof: Profile{DisplayName: strings.Repeat("n", 256)}, wantErr: ErrDisplayNameLong},
		{name: "empty attribute key", tenant: "t1", ext: "e", prof: Profile{Attributes: map[string]any{"": 1}}, wantErr: ErrBadAttributeKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewPlayer("id", tc.tenant, tc.ext, tc.prof, "", t0)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantExt, p.ExternalID)
			require.Equal(t, tc.wantMail, p.Email)
			require.True(t, p.Active, "new players are active")
			require.NotNil(t, p.Attributes, "attributes is always an object")
			require.Equal(t, t0, p.CreatedAt)
		})
	}
}

func TestNewPlayerRejectsTooManyAttributes(t *testing.T) {
	attrs := map[string]any{}
	for i := range MaxAttributes + 1 {
		attrs[fmt.Sprintf("k%d", i)] = i
	}
	_, err := NewPlayer("id", "t1", "e", Profile{Attributes: attrs}, "", t0)
	require.ErrorIs(t, err, ErrTooManyAttributes)
}

func basePlayer(t *testing.T) Player {
	t.Helper()
	p, err := NewPlayer("id", "t1", "ext", Profile{
		DisplayName: "Neo",
		Email:       "neo@matrix.io",
		Attributes:  map[string]any{"tier": "gold", "age": float64(30)},
	}, "", t0)
	require.NoError(t, err)
	return p
}

func TestApplyPatch(t *testing.T) {
	later := t0.Add(time.Hour)
	cases := []struct {
		name        string
		patch       Patch
		wantChanged []string
		check       func(t *testing.T, p Player)
	}{
		{
			name:  "empty patch changes nothing",
			patch: Patch{},
			check: func(t *testing.T, p Player) {
				require.Equal(t, t0, p.UpdatedAt, "no change, no timestamp bump")
			},
		},
		{
			name:        "only email: omitted fields untouched",
			patch:       Patch{Email: ptr("Trinity@Matrix.io")},
			wantChanged: []string{FieldEmail},
			check: func(t *testing.T, p Player) {
				require.Equal(t, "trinity@matrix.io", p.Email)
				require.Equal(t, "Neo", p.DisplayName)
				require.Equal(t, map[string]any{"tier": "gold", "age": float64(30)}, p.Attributes)
				require.Equal(t, later, p.UpdatedAt)
			},
		},
		{
			name:        "empty string clears display name",
			patch:       Patch{DisplayName: ptr("")},
			wantChanged: []string{FieldDisplayName},
			check:       func(t *testing.T, p Player) { require.Empty(t, p.DisplayName) },
		},
		{
			name:  "same value is not a change",
			patch: Patch{DisplayName: ptr("Neo"), Email: ptr("NEO@matrix.io")},
		},
		{
			name:        "attributes merge: set, delete with null, keep unlisted",
			patch:       Patch{Attributes: map[string]any{"tier": "platinum", "age": nil, "city": "Zion"}},
			wantChanged: []string{FieldAttributes},
			check: func(t *testing.T, p Player) {
				require.Equal(t, map[string]any{"tier": "platinum", "city": "Zion"}, p.Attributes)
			},
		},
		{
			name:  "attributes merge with identical values is not a change",
			patch: Patch{Attributes: map[string]any{"tier": "gold"}},
		},
		{
			name:        "is_active flip",
			patch:       Patch{Active: ptr(false)},
			wantChanged: []string{FieldActive},
			check:       func(t *testing.T, p Player) { require.False(t, p.Active) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := basePlayer(t)
			changed, err := p.ApplyPatch(tc.patch, later)
			require.NoError(t, err)
			require.Equal(t, tc.wantChanged, changed)
			if tc.check != nil {
				tc.check(t, p)
			}
		})
	}
}

func TestApplyPatchInvalidLeavesPlayerUntouched(t *testing.T) {
	p := basePlayer(t)
	before := p
	_, err := p.ApplyPatch(Patch{DisplayName: ptr("Morpheus"), Email: ptr("broken")}, t0.Add(time.Hour))
	require.ErrorIs(t, err, ErrEmailInvalid)
	require.Equal(t, before, p, "a rejected patch must not half-apply")
}

func TestActivateDeactivateReportChange(t *testing.T) {
	p := basePlayer(t)
	require.False(t, p.Activate(t0), "already active")
	require.True(t, p.Deactivate(t0.Add(time.Minute)))
	require.False(t, p.Deactivate(t0.Add(2*time.Minute)), "already inactive")
	require.Equal(t, t0.Add(time.Minute), p.UpdatedAt)
	require.True(t, p.Activate(t0.Add(3*time.Minute)))
	require.True(t, p.Active)
}

func TestMarkDeletedAndBelongsTo(t *testing.T) {
	p := basePlayer(t)
	require.True(t, p.BelongsTo("t1"))
	require.False(t, p.BelongsTo("t2"))
	require.False(t, p.BelongsTo(""))
	require.False(t, p.Deleted())
	p.MarkDeleted(t0)
	require.True(t, p.Deleted())
}

func TestSortNameFallsBackToExternalID(t *testing.T) {
	require.Equal(t, "neo", Player{DisplayName: "NEO", ExternalID: "X-1"}.SortName())
	require.Equal(t, "x-1", Player{ExternalID: "X-1"}.SortName())
}
