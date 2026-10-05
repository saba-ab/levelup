package pagination_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"myapp/internal/shared/errs"
	"myapp/internal/shared/pagination"
)

func TestCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 9, 1, 12, 30, 0, 123456000, time.UTC)
	cur := pagination.EncodeCursor(ts, "0192d3e0-aaaa-7bbb-cccc-ddddeeeeffff")

	gotTS, gotID, err := pagination.DecodeCursor(cur)
	require.NoError(t, err)
	require.True(t, ts.Equal(gotTS))
	require.Equal(t, "0192d3e0-aaaa-7bbb-cccc-ddddeeeeffff", gotID)
}

func TestDecodeGarbageIsInvalid(t *testing.T) {
	for _, bad := range []string{"", "!!!", "bm90LWEtY3Vyc29y", "MTIzNA"} {
		_, _, err := pagination.DecodeCursor(bad)
		require.Error(t, err, "input %q", bad)
		require.Equal(t, errs.Invalid, errs.KindOf(err))
	}
}
