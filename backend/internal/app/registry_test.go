package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/platform/modkit"
)

type stubModule struct {
	modkit.Module
	name string
}

func (s stubModule) Name() string { return s.name }

func ctors(names ...string) map[string]func() modkit.Module {
	out := map[string]func() modkit.Module{}
	for _, n := range names {
		out[n] = func() modkit.Module { return stubModule{name: n} }
	}
	return out
}

func TestSelectModulesFiltersAndOrders(t *testing.T) {
	got, err := selectModules(ctors("user", "wallet", "notification"), []string{"wallet", "user"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "wallet", got[0].Name())
	require.Equal(t, "user", got[1].Name())
}

func TestSelectModulesUnknownNameFailsFast(t *testing.T) {
	_, err := selectModules(ctors("user"), []string{"user", "billing"})
	require.Error(t, err)
	require.Contains(t, err.Error(), `"billing"`)
}

func TestSelectModulesRejectsDuplicates(t *testing.T) {
	_, err := selectModules(ctors("user"), []string{"user", "user"})
	require.Error(t, err)
}
