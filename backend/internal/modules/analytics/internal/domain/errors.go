package domain

import "levelup/internal/shared/errs"

// Problem codes (ADR-0016) clients may branch on.
const (
	CodeInvalidRange  = "analytics_invalid_range"
	CodeRangeTooLarge = "analytics_range_too_large"
	CodeInvalidCohort = "analytics_invalid_cohort"
	CodeInvalidSteps  = "analytics_invalid_funnel_steps"
)

var (
	ErrRangeInverted = errs.WithCode(errs.New(errs.Invalid, "from must not be after to"), CodeInvalidRange)
	ErrRangeTooLarge = errs.WithCode(errs.New(errs.Invalid, "the range may span at most 366 days"), CodeRangeTooLarge)
	ErrBadCohort     = errs.WithCode(errs.New(errs.Invalid, "cohort must be \"week\""), CodeInvalidCohort)
	ErrBadWeeks      = errs.WithCode(errs.New(errs.Invalid, "weeks must be between 1 and 52"), CodeInvalidCohort)
	ErrBadSteps      = errs.WithCode(errs.New(errs.Invalid, "steps must list 2 to 10 comma-separated event types of at most 100 characters"), CodeInvalidSteps)

	ErrFactNoEvent  = errs.New(errs.Invalid, "fact without event id")
	ErrFactNoTenant = errs.New(errs.Invalid, "fact without tenant id")
	ErrFactNoDay    = errs.New(errs.Invalid, "fact without a time")
)
