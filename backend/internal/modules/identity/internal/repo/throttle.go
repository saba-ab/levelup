package repo

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/shared/errs"
)

// Throttle is a fixed-window counter on redis-core: INCR, and the first
// hit of a window sets its TTL (every key expires: redis-core runs
// noeviction). Keys are namespaced "identity:throttle:".
type Throttle struct{ rdb goredis.UniversalClient }

func NewThrottle(rdb goredis.UniversalClient) *Throttle { return &Throttle{rdb: rdb} }

var _ app.Throttle = (*Throttle)(nil)

func (t *Throttle) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	k := "identity:throttle:" + key
	var incr *goredis.IntCmd
	_, err := t.rdb.TxPipelined(ctx, func(p goredis.Pipeliner) error {
		incr = p.Incr(ctx, k)
		p.ExpireNX(ctx, k, window)
		return nil
	})
	if err != nil {
		return false, errs.Wrap(errs.Unavailable, "throttle", err)
	}
	return incr.Val() <= int64(limit), nil
}
