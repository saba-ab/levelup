package id_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"levelup/internal/shared/id"
)

func TestNewIDIsUUIDv7(t *testing.T) {
	raw := id.NewID()
	parsed, err := uuid.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, uuid.Version(7), parsed.Version())
}

func TestNewIDIsUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		v := id.NewID()
		_, dup := seen[v]
		require.False(t, dup, "duplicate id %s", v)
		seen[v] = struct{}{}
	}
}

func TestDeriveIsDeterministicAndPartSensitive(t *testing.T) {
	a := id.Derive("activity-1", "rule-v1", "0")
	require.Equal(t, a, id.Derive("activity-1", "rule-v1", "0"))
	require.NotEqual(t, a, id.Derive("activity-1", "rule-v1", "1"))
	require.NotEqual(t, id.Derive("ab", "c"), id.Derive("a", "bc"))
}
