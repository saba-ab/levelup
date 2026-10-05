package money_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/money"
)

func TestAddAndSub(t *testing.T) {
	a := money.Amount(1500)
	sum, err := a.Add(500)
	require.NoError(t, err)
	require.Equal(t, money.Amount(2000), sum)

	diff, err := sum.Sub(2000)
	require.NoError(t, err)
	require.Equal(t, money.Amount(0), diff)
}

func TestAddOverflow(t *testing.T) {
	a := money.Amount(math.MaxInt64)
	_, err := a.Add(1)
	require.Error(t, err)

	b := money.Amount(math.MinInt64)
	_, err = b.Sub(1)
	require.Error(t, err)
}

func TestNegativeAllowedByArithmetic(t *testing.T) {
	// Whether a balance may go negative is a domain invariant, not money's job.
	a := money.Amount(100)
	diff, err := a.Sub(200)
	require.NoError(t, err)
	require.Equal(t, money.Amount(-100), diff)
}
