package id_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"myapp/internal/shared/id"
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
