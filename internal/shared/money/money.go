// Package money represents amounts as int64 minor units (cents), never float
// (PRD §5). Arithmetic is overflow-checked; sign rules are domain invariants
// and deliberately not enforced here.
package money

import (
	"myapp/internal/shared/errs"
)

type Amount int64

func (a Amount) Add(b Amount) (Amount, error) {
	sum := a + b
	if (b > 0 && sum < a) || (b < 0 && sum > a) {
		return 0, errs.New(errs.Invalid, "amount overflow")
	}
	return sum, nil
}

func (a Amount) Sub(b Amount) (Amount, error) {
	diff := a - b
	if (b < 0 && diff < a) || (b > 0 && diff > a) {
		return 0, errs.New(errs.Invalid, "amount overflow")
	}
	return diff, nil
}

func (a Amount) Minor() int64 { return int64(a) }
