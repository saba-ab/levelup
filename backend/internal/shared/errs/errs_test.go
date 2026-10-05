package errs_test

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

func TestNewCarriesKindAndMessage(t *testing.T) {
	err := errs.New(errs.NotFound, "wallet missing")
	require.EqualError(t, err, "wallet missing")
	require.Equal(t, errs.NotFound, errs.KindOf(err))
}

func TestWrapPreservesChain(t *testing.T) {
	err := errs.Wrap(errs.Unavailable, "redis down", io.ErrClosedPipe)
	require.ErrorIs(t, err, io.ErrClosedPipe)
	require.Equal(t, errs.Unavailable, errs.KindOf(err))
	require.Contains(t, err.Error(), "redis down")
	require.Contains(t, err.Error(), io.ErrClosedPipe.Error())
}

func TestKindOfWalksWrappedChains(t *testing.T) {
	inner := errs.Wrap(errs.NotFound, "no row", errors.New("sql: no rows"))
	outer := fmt.Errorf("loading user: %w", inner)
	require.Equal(t, errs.NotFound, errs.KindOf(outer))
}

func TestKindOfOutermostWins(t *testing.T) {
	inner := errs.New(errs.NotFound, "gone")
	outer := errs.Wrap(errs.Invalid, "bad request", inner)
	require.Equal(t, errs.Invalid, errs.KindOf(outer))
}

func TestKindOfUnknownCases(t *testing.T) {
	require.Equal(t, errs.Unknown, errs.KindOf(nil))
	require.Equal(t, errs.Unknown, errs.KindOf(errors.New("plain")))
}

func TestFieldsRoundTrip(t *testing.T) {
	err := errs.New(errs.Invalid, "validation failed")
	err = errs.WithFields(err, map[string]string{"amount": "must be > 0"})
	wrapped := fmt.Errorf("handler: %w", err)

	require.Equal(t, map[string]string{"amount": "must be > 0"}, errs.FieldsOf(wrapped))
	require.Equal(t, errs.Invalid, errs.KindOf(wrapped))
}

func TestFieldsOfWithoutFields(t *testing.T) {
	require.Nil(t, errs.FieldsOf(errors.New("plain")))
	require.Nil(t, errs.FieldsOf(nil))
}

func TestWithFieldsOnPlainError(t *testing.T) {
	err := errs.WithFields(errors.New("plain"), map[string]string{"f": "bad"})
	require.Equal(t, map[string]string{"f": "bad"}, errs.FieldsOf(err))
	require.Equal(t, errs.Unknown, errs.KindOf(err))
}

func TestKindString(t *testing.T) {
	require.Equal(t, "not_found", errs.NotFound.String())
	require.Equal(t, "invalid", errs.Invalid.String())
	require.Equal(t, "unknown", errs.Unknown.String())
}

func TestCodeSurvivesWrapping(t *testing.T) {
	base := errs.WithCode(errs.New(errs.Invalid, "insufficient balance"), "insufficient_balance")
	wrapped := fmt.Errorf("debit: %w", base)
	require.Equal(t, "insufficient_balance", errs.CodeOf(wrapped))
	require.Equal(t, errs.Invalid, errs.KindOf(wrapped))
	require.Empty(t, errs.CodeOf(errs.New(errs.Invalid, "no code")))
	require.Empty(t, errs.CodeOf(nil))
}
