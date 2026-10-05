package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// releaseScript is compare-and-delete (PRD §7.8.3): a plain DEL from a
// holder whose TTL already expired would release the CURRENT holder's lock,
// and two jobs run — the exact failure the lock exists to prevent.
var releaseScript = redis.NewScript(`
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		return redis.call("DEL", KEYS[1])
	end
	return 0`)

// Locker takes redis-core: a lock evicted by an LRU cache instance is a
// correctness incident (PRD §7.8).
type Locker struct {
	rdb redis.UniversalClient
	log *zap.Logger
}

func NewLocker(core Core, log *zap.Logger) *Locker {
	return &Locker{rdb: core.UniversalClient, log: log}
}

type Lock struct {
	rdb   redis.UniversalClient
	key   string
	token string
}

// Acquire FAILS CLOSED (R43): any error — held, or Redis unreachable — means
// the caller must not run. Set ttl comfortably above the job's own timeout;
// give the job a context deadline shorter than ttl (PRD §7.8.3).
func (l *Locker) Acquire(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	token := id.NewID()
	ok, err := l.rdb.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, errs.Wrap(errs.Unavailable, "lock backend unreachable — not running", err)
	}
	if !ok {
		return nil, errs.New(errs.Conflict, "lock held: "+key)
	}
	return &Lock{rdb: l.rdb, key: key, token: token}, nil
}

// Release deletes the lock only if this holder still owns it.
func (l *Lock) Release(ctx context.Context) error {
	return releaseScript.Run(ctx, l.rdb, []string{l.key}, l.token).Err()
}
