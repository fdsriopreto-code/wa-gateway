// Package cache encapsula o Redis: locks de posse de sessao, cache com TTL
// e pub/sub (para fan-out de WebSocket entre nos).
package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	c *redis.Client
}

func New(ctx context.Context, url string) (*Redis, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(opt)
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &Redis{c: c}, nil
}

func (r *Redis) Close() error       { return r.c.Close() }
func (r *Redis) Raw() *redis.Client { return r.c }

// ---- locks ----

var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
else
  return 0
end`)

var renewScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
  return 0
end`)

func (r *Redis) AcquireLock(ctx context.Context, key, val string, ttl time.Duration) (bool, error) {
	return r.c.SetNX(ctx, key, val, ttl).Result()
}

func (r *Redis) RenewLock(ctx context.Context, key, val string, ttl time.Duration) (bool, error) {
	res, err := renewScript.Run(ctx, r.c, []string{key}, val, ttl.Milliseconds()).Int64()
	return res == 1, err
}

func (r *Redis) ReleaseLock(ctx context.Context, key, val string) error {
	return releaseScript.Run(ctx, r.c, []string{key}, val).Err()
}

// ---- cache com TTL ----

func (r *Redis) SetJSON(ctx context.Context, key string, v any, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return r.c.Set(ctx, key, b, ttl).Err()
}

func (r *Redis) GetJSON(ctx context.Context, key string, dst any) (bool, error) {
	b, err := r.c.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(b, dst)
}

// ---- pub/sub ----

func (r *Redis) Publish(ctx context.Context, channel string, payload []byte) error {
	return r.c.Publish(ctx, channel, payload).Err()
}

func (r *Redis) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return r.c.Subscribe(ctx, channels...)
}
