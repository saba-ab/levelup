package repo

import (
	"context"
	"errors"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"levelup/internal/modules/leaderboards/internal/app"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/shared/errs"
)

// Redis is the redis-core read model. One sorted set per board period:
//
//	lb:{<leaderboard_id>:<yyyymmdd>}        members = player ids
//	lb:{<leaderboard_id>:<yyyymmdd>}:ready  marker: the set holds the full ranking
//
// Members are stored with the NEGATED score so the natural ascending order
// (score asc, member asc) is exactly the ranking order (score desc, player id
// asc) — ZREVRANGE would break ties by player id descending. The hash tag
// keeps both keys in one cluster slot for the Lua scripts and RENAME.
// Every key carries a TTL: redis-core runs noeviction.
type Redis struct {
	rdb goredis.UniversalClient
}

func NewRedis(rdb goredis.UniversalClient) *Redis { return &Redis{rdb: rdb} }

var _ app.RankStore = (*Redis)(nil)

const zaddChunk = 1000

func setKey(p domain.Period) string {
	return "lb:{" + p.LeaderboardID + ":" + p.Start.UTC().Format("20060102") + "}"
}

func readyKey(p domain.Period) string { return setKey(p) + ":ready" }
func tmpKey(p domain.Period) string   { return setKey(p) + ":tmp" }
func lockKey(p domain.Period) string  { return setKey(p) + ":warming" }

// Writes apply only to a loaded ("ready") set, inherit the marker's TTL when
// they create the set, and never create a partial ranking.
var (
	incrScript = goredis.NewScript(`
local ttl = redis.call('PTTL', KEYS[2])
if ttl <= 0 then return 0 end
redis.call('ZINCRBY', KEYS[1], ARGV[1], ARGV[2])
if redis.call('PTTL', KEYS[1]) == -1 then redis.call('PEXPIRE', KEYS[1], ttl) end
return 1`)
	setScript = goredis.NewScript(`
local ttl = redis.call('PTTL', KEYS[2])
if ttl <= 0 then return 0 end
redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
if redis.call('PTTL', KEYS[1]) == -1 then redis.call('PEXPIRE', KEYS[1], ttl) end
return 1`)
)

func wrap(msg string, err error) error {
	if err == nil {
		return nil
	}
	return errs.Wrap(errs.Unavailable, msg, err)
}

func (r *Redis) Ready(ctx context.Context, p domain.Period) (bool, error) {
	n, err := r.rdb.Exists(ctx, readyKey(p)).Result()
	if err != nil {
		return false, wrap("leaderboard ready marker", err)
	}
	return n == 1, nil
}

func (r *Redis) Increment(ctx context.Context, p domain.Period, playerID string, delta int64) error {
	err := incrScript.Run(ctx, r.rdb, []string{setKey(p), readyKey(p)}, -delta, playerID).Err()
	return wrap("leaderboard increment", err)
}

func (r *Redis) Set(ctx context.Context, p domain.Period, playerID string, score int64) error {
	err := setScript.Run(ctx, r.rdb, []string{setKey(p), readyKey(p)}, -score, playerID).Err()
	return wrap("leaderboard set", err)
}

func (r *Redis) Remove(ctx context.Context, p domain.Period, playerIDs ...string) error {
	if len(playerIDs) == 0 {
		return nil
	}
	members := make([]any, len(playerIDs))
	for i, id := range playerIDs {
		members[i] = id
	}
	return wrap("leaderboard remove", r.rdb.ZRem(ctx, setKey(p), members...).Err())
}

func (r *Redis) Range(ctx context.Context, p domain.Period, start, stop int64) ([]domain.Standing, error) {
	zs, err := r.rdb.ZRangeWithScores(ctx, setKey(p), start, stop).Result()
	if err != nil {
		return nil, wrap("leaderboard range", err)
	}
	out := make([]domain.Standing, len(zs))
	for i, z := range zs {
		member, _ := z.Member.(string)
		out[i] = domain.Standing{PlayerID: member, Score: -int64(z.Score)}
	}
	return out, nil
}

func (r *Redis) Position(ctx context.Context, p domain.Period, playerID string) (int64, int64, bool, error) {
	pipe := r.rdb.Pipeline()
	rank := pipe.ZRank(ctx, setKey(p), playerID)
	score := pipe.ZScore(ctx, setKey(p), playerID)
	_, err := pipe.Exec(ctx)
	if errors.Is(err, goredis.Nil) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, wrap("leaderboard position", err)
	}
	return rank.Val(), -int64(score.Val()), true, nil
}

func (r *Redis) CountAbove(ctx context.Context, p domain.Period, score int64) (int64, error) {
	n, err := r.rdb.ZCount(ctx, setKey(p), "-inf", "("+strconv.FormatInt(-score, 10)).Result()
	if err != nil {
		return 0, wrap("leaderboard count", err)
	}
	return n, nil
}

// Replace builds the set under a temporary key and swaps it in with the
// ready marker in one MULTI, so readers never see a half-built ranking.
func (r *Redis) Replace(ctx context.Context, p domain.Period, rows []domain.Standing, expireAt time.Time) error {
	key, ready, tmp := setKey(p), readyKey(p), tmpKey(p)
	_, err := r.rdb.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
		pipe.Del(ctx, tmp)
		for i := 0; i < len(rows); i += zaddChunk {
			chunk := rows[i:min(i+zaddChunk, len(rows))]
			zs := make([]goredis.Z, len(chunk))
			for j, row := range chunk {
				zs[j] = goredis.Z{Score: float64(-row.Score), Member: row.PlayerID}
			}
			pipe.ZAdd(ctx, tmp, zs...)
		}
		if len(rows) > 0 {
			pipe.Rename(ctx, tmp, key)
			pipe.ExpireAt(ctx, key, expireAt)
		} else {
			pipe.Del(ctx, key)
		}
		pipe.Set(ctx, ready, "1", 0)
		pipe.ExpireAt(ctx, ready, expireAt)
		return nil
	})
	return wrap("leaderboard replace", err)
}

// Delete drops the keys of the given periods. Keys of different periods
// live in different slots, so this is a plain (non-transactional) pipeline.
func (r *Redis) Delete(ctx context.Context, periods ...domain.Period) error {
	if len(periods) == 0 {
		return nil
	}
	pipe := r.rdb.Pipeline()
	for _, p := range periods {
		pipe.Unlink(ctx, setKey(p), readyKey(p))
	}
	_, err := pipe.Exec(ctx)
	return wrap("leaderboard delete", err)
}

func (r *Redis) TryLock(ctx context.Context, p domain.Period, ttl time.Duration) (bool, error) {
	ok, err := r.rdb.SetNX(ctx, lockKey(p), "1", ttl).Result()
	if err != nil {
		return false, wrap("leaderboard warm lock", err)
	}
	return ok, nil
}

// Disabled is the RankStore used when no redis-core client is wired: never
// ready, so every read is served from Postgres.
type Disabled struct{}

var _ app.RankStore = Disabled{}

func (Disabled) Ready(context.Context, domain.Period) (bool, error)              { return false, nil }
func (Disabled) Increment(context.Context, domain.Period, string, int64) error   { return nil }
func (Disabled) Set(context.Context, domain.Period, string, int64) error         { return nil }
func (Disabled) Remove(context.Context, domain.Period, ...string) error          { return nil }
func (Disabled) CountAbove(context.Context, domain.Period, int64) (int64, error) { return 0, nil }
func (Disabled) Delete(context.Context, ...domain.Period) error                  { return nil }
func (Disabled) TryLock(context.Context, domain.Period, time.Duration) (bool, error) {
	return false, nil
}
func (Disabled) Range(context.Context, domain.Period, int64, int64) ([]domain.Standing, error) {
	return nil, nil
}
func (Disabled) Position(context.Context, domain.Period, string) (int64, int64, bool, error) {
	return 0, 0, false, nil
}
func (Disabled) Replace(context.Context, domain.Period, []domain.Standing, time.Time) error {
	return nil
}
