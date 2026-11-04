// Package redisstore is an OUTBOUND ADAPTER over Redis: refresh-token
// sessions, login rate limiting and the user cache.
package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
	"github.com/yourorg/wallet-service/services/auth/internal/service"
)

type SessionStore struct{ rdb redis.UniversalClient }

func NewSessionStore(rdb redis.UniversalClient) *SessionStore { return &SessionStore{rdb: rdb} }

func sessionKey(id string) string { return "auth:session:" + id }

func (s *SessionStore) Create(ctx context.Context, sess model.Session, ttl time.Duration) error {
	k := sessionKey(sess.ID)
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, k, "user_id", sess.UserID, "role", sess.Role, "secret", sess.SecretHash)
	pipe.Expire(ctx, k, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis create session: %w", err)
	}
	return nil
}

// Compare-and-swap in one Lua call so two concurrent refreshes with the same
// token cannot both succeed. Returns {status, user_id, role}: 1 ok, 0 missing, -1 mismatch.
var rotateScript = redis.NewScript(`
local v = redis.call('HMGET', KEYS[1], 'secret', 'user_id', 'role')
if not v[1] then return {0} end
if v[1] ~= ARGV[1] then return {-1} end
redis.call('HSET', KEYS[1], 'secret', ARGV[2])
redis.call('EXPIRE', KEYS[1], ARGV[3])
return {1, v[2], v[3]}`)

func (s *SessionStore) Rotate(ctx context.Context, id, oldHash, newHash string, ttl time.Duration) (model.Session, error) {
	res, err := rotateScript.Run(ctx, s.rdb, []string{sessionKey(id)}, oldHash, newHash, int(ttl.Seconds())).Slice()
	if err != nil {
		return model.Session{}, fmt.Errorf("redis rotate session: %w", err)
	}
	switch res[0].(int64) {
	case 1:
		return model.Session{ID: id, UserID: res[1].(string), Role: res[2].(string), SecretHash: newHash}, nil
	case -1:
		return model.Session{}, service.ErrTokenReuse
	default:
		return model.Session{}, model.NewError(model.ErrUnauthenticated, "session not found")
	}
}

func (s *SessionStore) Active(ctx context.Context, id string) (bool, error) {
	n, err := s.rdb.Exists(ctx, sessionKey(id)).Result()
	if err != nil {
		return false, fmt.Errorf("redis session exists: %w", err)
	}
	return n == 1, nil
}

func (s *SessionStore) Revoke(ctx context.Context, id string) error {
	if err := s.rdb.Del(ctx, sessionKey(id)).Err(); err != nil {
		return fmt.Errorf("redis revoke session: %w", err)
	}
	return nil
}
