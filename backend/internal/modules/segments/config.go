package segments

import "time"

// Config is segments' settings; the composition root embeds it with
// envPrefix "SEGMENTS_".
type Config struct {
	// RefreshSchedule is the cron spec (UTC) of segments.refresh.
	RefreshSchedule string `env:"REFRESH_SCHEDULE" envDefault:"15 * * * *"`
	// PageSize is how many players a refresh evaluates per transaction.
	PageSize int `env:"PAGE_SIZE" envDefault:"500"`
	// RefreshLease bounds how long a crashed refresh run blocks the next.
	RefreshLease time.Duration `env:"REFRESH_LEASE" envDefault:"15m"`
	// PreviewLimit is how many players POST /segments/preview evaluates.
	PreviewLimit int `env:"PREVIEW_LIMIT" envDefault:"1000"`
}
