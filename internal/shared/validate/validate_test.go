package validate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"myapp/internal/shared/errs"
	"myapp/internal/shared/validate"
)

type withdrawReq struct {
	Amount   int64  `json:"amount"   validate:"required,gt=0"`
	Currency string `json:"currency" validate:"required,iso4217"`
}

func TestStructPassesValidInput(t *testing.T) {
	v := validate.New()
	require.NoError(t, v.Struct(withdrawReq{Amount: 100, Currency: "USD"}))
}

func TestStructMapsFailuresToInvalidWithJSONFieldNames(t *testing.T) {
	v := validate.New()
	err := v.Struct(withdrawReq{Amount: -5, Currency: "notacurrency"})

	require.Error(t, err)
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	fields := errs.FieldsOf(err)
	require.Contains(t, fields, "amount")
	require.Contains(t, fields, "currency")
}

func TestStructZeroValueRequired(t *testing.T) {
	v := validate.New()
	err := v.Struct(withdrawReq{})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	require.Len(t, errs.FieldsOf(err), 2)
}
