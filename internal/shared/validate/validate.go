// Package validate wraps go-playground/validator so nothing outside this
// package imports it (PRD §7.5): transport DTOs get shape validation here,
// and failures surface as errs.Invalid with a per-field map that httpx can
// render as problem+json without knowing the validator exists.
package validate

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	"myapp/internal/shared/errs"
)

type Validator struct {
	v *validator.Validate
}

func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())

	// Report json field names, not Go field names — errors face the API client.
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" || name == "" {
			return fld.Name
		}
		return name
	})

	return &Validator{v: v}
}

// Struct validates shape only. Domain invariants belong in constructors
// (PRD §7.5: "tags validate shape, constructors validate invariants").
func (val *Validator) Struct(s any) error {
	err := val.v.Struct(s)
	if err == nil {
		return nil
	}

	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return errs.Wrap(errs.Internal, "validator", err)
	}

	fields := make(map[string]string, len(verrs))
	for _, fe := range verrs {
		fields[fe.Field()] = messageFor(fe)
	}
	return errs.WithFields(errs.New(errs.Invalid, "validation failed"), fields)
}

func messageFor(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "gt":
		return fmt.Sprintf("must be greater than %s", fe.Param())
	case "gte":
		return fmt.Sprintf("must be at least %s", fe.Param())
	case "lt":
		return fmt.Sprintf("must be less than %s", fe.Param())
	case "lte":
		return fmt.Sprintf("must be at most %s", fe.Param())
	case "min":
		return fmt.Sprintf("must have at least %s", fe.Param())
	case "max":
		return fmt.Sprintf("must have at most %s", fe.Param())
	case "email":
		return "must be a valid email address"
	case "uuid":
		return "must be a valid UUID"
	case "iso4217":
		return "must be a valid ISO 4217 currency code"
	default:
		return fmt.Sprintf("failed %q validation", fe.Tag())
	}
}
