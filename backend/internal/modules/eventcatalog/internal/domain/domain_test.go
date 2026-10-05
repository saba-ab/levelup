package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func TestCreateEventTypeInvariants(t *testing.T) {
	cases := []struct {
		name     string
		in       NewEventType
		wantErr  error
		wantSlug string
	}{
		{"derives snake_case slug from name", NewEventType{Name: "My Custom Event"}, nil, "my_custom_event"},
		{"collapses punctuation runs", NewEventType{Name: "  Checkout -- Done!! "}, nil, "checkout_done"},
		{"explicit kebab slug honoured", NewEventType{Name: "x", Slug: "my-custom-event"}, nil, "my-custom-event"},
		{"explicit snake slug honoured", NewEventType{Name: "x", Slug: "purchase_completed"}, nil, "purchase_completed"},
		{"blank name", NewEventType{Name: "   "}, ErrInvalidName, ""},
		{"name too long", NewEventType{Name: strings.Repeat("a", 256)}, ErrInvalidName, ""},
		{"name of 255 runes ok", NewEventType{Name: strings.Repeat("ა", 255), Slug: "georgian"}, nil, "georgian"},
		{"underivable slug", NewEventType{Name: "ყიდვა"}, ErrInvalidSlug, ""},
		{"uppercase slug", NewEventType{Name: "x", Slug: "Purchase"}, ErrInvalidSlug, ""},
		{"double separator", NewEventType{Name: "x", Slug: "a__b"}, ErrInvalidSlug, ""},
		{"leading separator", NewEventType{Name: "x", Slug: "_a"}, ErrInvalidSlug, ""},
		{"slug longer than rules.trigger_event", NewEventType{Name: "x", Slug: strings.Repeat("a", 101)}, ErrInvalidSlug, ""},
		{"description too long", NewEventType{Name: "x", Description: strings.Repeat("d", 1001)}, ErrInvalidDescription, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			et, err := CreateEventType(tc.in, now)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantSlug, et.Slug)
			require.NotEmpty(t, et.ID)
			require.Equal(t, now, et.CreatedAt)
		})
	}
}

func TestSlugifyTruncatesToMaxLength(t *testing.T) {
	s := Slugify(strings.Repeat("ab ", 60))
	require.LessOrEqual(t, len(s), MaxSlugLen)
	require.NoError(t, CheckSlug(s))
}

func TestValidatePropertySchema(t *testing.T) {
	cases := []struct {
		name   string
		schema map[string]any
		ok     bool
	}{
		{"nil is free-form", nil, true},
		{"empty object", map[string]any{}, true},
		{"typical", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"amount":   map[string]any{"type": "number"},
				"currency": map[string]any{"type": "string", "title": "ISO code"},
			},
			"required": []any{"amount"},
		}, true},
		{"root type not object", map[string]any{"type": "array"}, false},
		{"properties not object", map[string]any{"properties": []any{}}, false},
		{"property def not object", map[string]any{"properties": map[string]any{"a": "string"}}, false},
		{"unknown property type", map[string]any{"properties": map[string]any{"a": map[string]any{"type": "date"}}}, false},
		{"required not array", map[string]any{"required": "a"}, false},
		{"required undeclared", map[string]any{"properties": map[string]any{}, "required": []any{"a"}}, false},
		{"required non-string", map[string]any{"properties": map[string]any{}, "required": []any{1.0}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePropertySchema(tc.schema)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestApplyPatch(t *testing.T) {
	base := func() EventType {
		et, err := CreateEventType(NewEventType{
			TenantID: "t1", Name: "Checkout", Description: "keep me", CategoryID: "c1",
			PropertySchema: map[string]any{"type": "object"}, Active: true,
		}, now)
		require.NoError(t, err)
		return et
	}
	later := now.Add(time.Hour)

	cases := []struct {
		name       string
		patch      EventTypePatch
		wantErr    error
		wantAnyErr bool
		check      func(t *testing.T, before, after EventType)
	}{
		{"empty patch keeps everything", EventTypePatch{}, nil, false, func(t *testing.T, b, a EventType) {
			b.UpdatedAt = later
			require.Equal(t, b, a)
		}},
		{"partial name keeps description", EventTypePatch{Name: ptr("Checkout v2")}, nil, false, func(t *testing.T, _, a EventType) {
			require.Equal(t, "Checkout v2", a.Name)
			require.Equal(t, "keep me", a.Description)
			require.Equal(t, "c1", a.CategoryID)
		}},
		{"description can be cleared", EventTypePatch{Description: ptr("")}, nil, false, func(t *testing.T, _, a EventType) {
			require.Empty(t, a.Description)
		}},
		{"category can be cleared", EventTypePatch{CategoryID: ptr("")}, nil, false, func(t *testing.T, _, a EventType) {
			require.Empty(t, a.CategoryID)
		}},
		{"schema cleared", EventTypePatch{SetSchema: true}, nil, false, func(t *testing.T, _, a EventType) {
			require.Nil(t, a.PropertySchema)
		}},
		{"deactivate", EventTypePatch{Active: ptr(false)}, nil, false, func(t *testing.T, _, a EventType) {
			require.False(t, a.Active)
		}},
		{"same slug is a no-op", EventTypePatch{Slug: ptr("checkout")}, nil, false, nil},
		{"slug change refused", EventTypePatch{Slug: ptr("checkout_v2")}, ErrSlugImmutable, false, nil},
		{"blank name refused", EventTypePatch{Name: ptr(" ")}, ErrInvalidName, false, nil},
		{"bad schema refused", EventTypePatch{SetSchema: true, PropertySchema: map[string]any{"type": "string"}}, nil, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := base()
			after := before
			err := after.Apply(tc.patch, later)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, before, after, "a refused patch must not change the entity")
			case tc.wantAnyErr:
				require.Error(t, err)
				require.Equal(t, before, after)
			default:
				require.NoError(t, err)
				require.Equal(t, later, after.UpdatedAt)
				if tc.check != nil {
					tc.check(t, before, after)
				}
			}
		})
	}
}

func TestVisibilityAndShadowing(t *testing.T) {
	global := EventType{ID: "g", Slug: "login"}
	own := EventType{ID: "o", TenantID: "t1", Slug: "login"}
	foreign := EventType{ID: "f", TenantID: "t2", Slug: "checkout"}
	onlyGlobal := EventType{ID: "g2", Slug: "level_up"}

	require.True(t, global.VisibleTo("t1"))
	require.True(t, own.VisibleTo("t1"))
	require.False(t, foreign.VisibleTo("t1"))

	got := ResolveBySlug("t1", []EventType{global, own, foreign, onlyGlobal})
	require.Len(t, got, 2)
	require.Equal(t, "o", got["login"].ID, "tenant row shadows the global one")
	require.Equal(t, "g2", got["level_up"].ID)

	// Order must not matter.
	got = ResolveBySlug("t1", []EventType{own, global})
	require.Equal(t, "o", got["login"].ID)

	// Without a tenant only globals resolve.
	got = ResolveBySlug("", []EventType{own, global})
	require.Equal(t, "g", got["login"].ID)
}

func TestCategoryInvariants(t *testing.T) {
	c, err := CreateCategory(NewCategory{Name: "Commerce", SortOrder: 20}, now)
	require.NoError(t, err)
	require.Equal(t, "commerce", c.Slug)
	require.True(t, c.IsGlobal())
	require.True(t, c.AssignableTo(""))
	require.True(t, c.AssignableTo("t1"))

	own := Category{TenantID: "t1"}
	require.True(t, own.AssignableTo("t1"))
	require.False(t, own.AssignableTo("t2"))
	require.False(t, own.AssignableTo(""), "a global type may not use a tenant category")

	_, err = CreateCategory(NewCategory{Name: ""}, now)
	require.ErrorIs(t, err, ErrInvalidName)

	require.ErrorIs(t, c.Apply(CategoryPatch{Slug: ptr("Bad Slug")}, now), ErrInvalidSlug)
	require.NoError(t, c.Apply(CategoryPatch{Slug: ptr("shop"), SortOrder: ptr(5)}, now))
	require.Equal(t, "shop", c.Slug)
	require.Equal(t, 5, c.SortOrder)
	require.Equal(t, "Commerce", c.Name)
}
