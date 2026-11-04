package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter is a fixed-window counter: INCR, and set the expiry on the first hit.
type RateLimiter struct{ rdb redis.UniversalClient }

func NewRateLimiter(rdb redis.UniversalClient) *RateLimiter { return &RateLimiter{rdb: rdb} }

var incrScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return n`)

func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	n, err := incrScript.Run(ctx, l.rdb, []string{"auth:rl:" + key}, window.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("redis rate limit: %w", err)
	}
	return n <= limit, nil
}
