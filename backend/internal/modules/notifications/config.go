package notifications

import "time"

// Config holds NOTIFICATIONS_* settings; the composition root embeds it
// with envPrefix "NOTIFICATIONS_".
type Config struct {
	// RenderTimeout bounds one template execution (title or body).
	RenderTimeout time.Duration `env:"RENDER_TIMEOUT" envDefault:"250ms"`
	// SendTimeout bounds one mailer call; the send lease is 1.5x this.
	SendTimeout time.Duration `env:"SEND_TIMEOUT" envDefault:"30s"`
	// EmailMaxAttempts settles an email as failed after this many transient
	// mailer errors. 4 = the first try + the 3 rungs of the retry ladder.
	EmailMaxAttempts int `env:"EMAIL_MAX_ATTEMPTS" envDefault:"4"`
	// EmailStaleAfter fails email rows still pending this long after
	// creation (job parked in the DLQ, worker crashed mid-send).
	EmailStaleAfter time.Duration `env:"EMAIL_STALE_AFTER" envDefault:"2h"`
	// SweepSchedule is the cron spec (UTC, 5 fields) of
	// notifications.sweep_stale_email.
	SweepSchedule string `env:"SWEEP_SCHEDULE" envDefault:"*/15 * * * *"`
}
