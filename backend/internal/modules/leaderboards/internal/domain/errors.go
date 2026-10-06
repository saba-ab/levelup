package domain

import "levelup/internal/shared/errs"

// Problem codes (ADR-0016) clients may branch on.
const (
	CodeNotFound           = "leaderboard_not_found"
	CodeSlugTaken          = "leaderboard_slug_taken"
	CodeVersionConflict    = "leaderboard_version_conflict"
	CodeInvalidType        = "leaderboard_invalid_type"
	CodeInvalidMetric      = "leaderboard_invalid_metric"
	CodeInvalidReset       = "leaderboard_invalid_reset_frequency"
	CodeBalanceNotPeriodic = "leaderboard_balance_requires_never"
	CodeInvalidPeriod      = "leaderboard_invalid_period"
	CodeInvalidConfig      = "leaderboard_invalid_config"
	CodePlayerNotFound     = "player_not_found"
	CodePlayerNotRanked    = "player_not_ranked"
)

var (
	ErrNotFound         = errs.WithCode(errs.New(errs.NotFound, "leaderboard not found"), CodeNotFound)
	ErrSlugTaken        = errs.WithCode(errs.New(errs.Conflict, "a leaderboard with this slug already exists"), CodeSlugTaken)
	ErrVersionConflict  = errs.WithCode(errs.New(errs.Conflict, "leaderboard modified concurrently, retry"), CodeVersionConflict)
	ErrNameRequired     = errs.New(errs.Invalid, "name is required")
	ErrSlugInvalid      = errs.New(errs.Invalid, "slug must be lower-case letters, digits and dashes")
	ErrTenantRequired   = errs.New(errs.Invalid, "tenant is required")
	ErrInvalidType      = errs.WithCode(errs.New(errs.Invalid, "type must be one of points, badges, missions, xp, activity"), CodeInvalidType)
	ErrConfigNotAllowed = errs.WithFields(errs.WithCode(errs.New(errs.Invalid, "config is only valid for activity leaderboards"), CodeInvalidConfig),
		map[string]string{"config": "only valid for type activity"})
	ErrInvalidMetric   = errs.WithCode(errs.New(errs.Invalid, "metric is not valid for this leaderboard type"), CodeInvalidMetric)
	ErrInvalidReset    = errs.WithCode(errs.New(errs.Invalid, "reset_frequency must be one of never, daily, weekly, monthly"), CodeInvalidReset)
	ErrBalancePeriodic = errs.WithCode(errs.New(errs.Invalid, "metric balance is only valid with reset_frequency never"), CodeBalanceNotPeriodic)
	ErrMaxEntries      = errs.New(errs.Invalid, "max_entries must be between 1 and 1000")
	ErrInvalidPeriod   = errs.WithCode(errs.New(errs.Invalid, "period must be 'current' or an RFC 3339 timestamp"), CodeInvalidPeriod)
	ErrPlayerNotFound  = errs.WithCode(errs.New(errs.NotFound, "player not found"), CodePlayerNotFound)
	ErrPlayerNotRanked = errs.WithCode(errs.New(errs.NotFound, "player has no score on this leaderboard for the period"), CodePlayerNotRanked)
)
