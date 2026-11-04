package redisstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

type UserCache struct{ rdb redis.UniversalClient }

func NewUserCache(rdb redis.UniversalClient) *UserCache { return &UserCache{rdb: rdb} }

// cachedUser deliberately omits the password hash: it never goes into Redis.
type cachedUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *UserCache) Get(ctx context.Context, id string) (model.User, bool, error) {
	b, err := c.rdb.Get(ctx, "auth:user:"+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return model.User{}, false, nil
	}
	if err != nil {
		return model.User{}, false, fmt.Errorf("redis get user: %w", err)
	}
	var cu cachedUser
	if err := json.Unmarshal(b, &cu); err != nil {
		return model.User{}, false, nil // treat corrupt entries as a miss
	}
	return model.User{ID: cu.ID, Email: cu.Email, Role: cu.Role, CreatedAt: cu.CreatedAt}, true, nil
}

func (c *UserCache) Set(ctx context.Context, u model.User, ttl time.Duration) error {
	b, _ := json.Marshal(cachedUser{ID: u.ID, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt})
	if err := c.rdb.Set(ctx, "auth:user:"+u.ID, b, ttl).Err(); err != nil {
		return fmt.Errorf("redis set user: %w", err)
	}
	return nil
}
