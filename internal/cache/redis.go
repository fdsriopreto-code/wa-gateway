// Package cache encapsula o Redis: locks de posse de sessao, cache com TTL
// e pub/sub (para fan-out de WebSocket entre nos).
package cache

import (
	"context"
	"encoding/json"
	"strconv"
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

// LockOwner devolve o valor atual do lock (ex.: o nodeID dono da sessão), ""
// se ninguém segura.
func (r *Redis) LockOwner(ctx context.Context, key string) (string, error) {
	v, err := r.c.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return v, err
}

// SetNodeAddr publica a URL HTTP alcançável deste nó (chave com TTL, renovada
// pelo heartbeat). GetNodeAddr resolve nodeID → URL.
func (r *Redis) SetNodeAddr(ctx context.Context, nodeID, url string, ttl time.Duration) error {
	return r.c.Set(ctx, "wa:node:addr:"+nodeID, url, ttl).Err()
}

func (r *Redis) GetNodeAddr(ctx context.Context, nodeID string) (string, error) {
	v, err := r.c.Get(ctx, "wa:node:addr:"+nodeID).Result()
	if err == redis.Nil {
		return "", nil
	}
	return v, err
}

func (r *Redis) RenewLock(ctx context.Context, key, val string, ttl time.Duration) (bool, error) {
	res, err := renewScript.Run(ctx, r.c, []string{key}, val, ttl.Milliseconds()).Int64()
	return res == 1, err
}

func (r *Redis) ReleaseLock(ctx context.Context, key, val string) error {
	return releaseScript.Run(ctx, r.c, []string{key}, val).Err()
}

// ---- rate limit (token bucket) ----

// rateScript e um token bucket: recarrega `rate` tokens/s ate `burst`, gasta
// `want` por chamada. Devolve {permitido(0/1), tokens_restantes, retry_ms}.
var rateScript = redis.NewScript(`
local rate  = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now   = tonumber(ARGV[3])
local want  = tonumber(ARGV[4])
local d = redis.call('HMGET', KEYS[1], 'tk', 'ts')
local tokens = tonumber(d[1])
local ts = tonumber(d[2])
if tokens == nil then tokens = burst; ts = now end
local delta = now - ts
if delta < 0 then delta = 0 end
tokens = math.min(burst, tokens + (delta / 1000.0) * rate)
local allowed = 0
local retry = 0
if tokens >= want then
  allowed = 1
  tokens = tokens - want
else
  retry = math.ceil((want - tokens) / rate * 1000)
end
redis.call('HSET', KEYS[1], 'tk', tokens, 'ts', now)
redis.call('PEXPIRE', KEYS[1], math.ceil(burst / rate * 1000) + 1000)
return {allowed, math.floor(tokens), retry}
`)

// RateAllow consome 1 token do bucket `key`. allowed=false quando estourou;
// retryAfter diz em quanto tempo haveria token de novo.
func (r *Redis) RateAllow(ctx context.Context, key string, rate, burst float64) (allowed bool, remaining int, retryAfter time.Duration, err error) {
	now := time.Now().UnixMilli()
	res, e := rateScript.Run(ctx, r.c, []string{"rl:" + key}, rate, burst, now, 1).Int64Slice()
	if e != nil {
		return true, 0, 0, e // fail-open: Redis fora do ar nao trava a API
	}
	if len(res) < 3 {
		return true, 0, 0, nil
	}
	return res[0] == 1, int(res[1]), time.Duration(res[2]) * time.Millisecond, nil
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

// ---- presenca de nos ----

// Heartbeat marca este no como vivo no sorted set `key` e devolve quantos
// nos foram vistos nos ultimos `ttl`. Usado pra so propagar eventos via
// pub/sub quando ha mais de um no.
func (r *Redis) Heartbeat(ctx context.Context, key, node string, ttl time.Duration) (int, error) {
	now := time.Now().UnixMilli()
	cutoff := strconv.FormatInt(now-ttl.Milliseconds(), 10)
	pipe := r.c.Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: node})
	pipe.ZRemRangeByScore(ctx, key, "-inf", "("+cutoff)
	card := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, ttl*4)
	if _, err := pipe.Exec(ctx); err != nil {
		return 1, err
	}
	return int(card.Val()), nil
}

// ActiveNodes devolve os membros do sorted set `key` vistos nos últimos `ttl`
// (nós vivos, pelo heartbeat).
func (r *Redis) ActiveNodes(ctx context.Context, key string, ttl time.Duration) ([]string, error) {
	min := strconv.FormatInt(time.Now().Add(-ttl).UnixMilli(), 10)
	return r.c.ZRangeByScore(ctx, key, &redis.ZRangeBy{Min: min, Max: "+inf"}).Result()
}

// ---- pub/sub ----

func (r *Redis) Publish(ctx context.Context, channel string, payload []byte) error {
	return r.c.Publish(ctx, channel, payload).Err()
}

func (r *Redis) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return r.c.Subscribe(ctx, channels...)
}
