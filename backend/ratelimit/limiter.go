// Package ratelimit provides a per-key fixed-window HTTP throttle.
// It is intentionally separate from cache.Cache — FetchCached and Allow are
// different seams that only share Redis as a transport.
package ratelimit

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter enforces a fixed-window request budget keyed by an opaque string
// (typically a client IP). No-ops (always allows) when Redis is unconfigured
// or unreachable — a Redis hiccup should throttle nothing, not 500 the API.
type Limiter struct {
	client *redis.Client
}

// New builds a Limiter from UPSTASH_REDIS_URL. Without a usable URL the
// returned Limiter always allows.
func New() *Limiter {
	redisURL := os.Getenv("UPSTASH_REDIS_URL")
	if redisURL == "" {
		return &Limiter{}
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: invalid UPSTASH_REDIS_URL, rate limiting disabled: %v\n", err)
		return &Limiter{}
	}
	opt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &Limiter{client: redis.NewClient(opt)}
}

// Allow reports whether a request identified by key is within limit for the
// given fixed window, incrementing key's counter as a side effect.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error) {
	if l == nil || l.client == nil {
		return true, 0, nil
	}

	rlKey := "ratelimit:" + key
	pipe := l.client.Pipeline()
	incr := pipe.Incr(ctx, rlKey)
	pipe.ExpireNX(ctx, rlKey, window) // only takes effect on the first request in a window
	if _, err := pipe.Exec(ctx); err != nil {
		return true, 0, err
	}

	if incr.Val() > int64(limit) {
		ttl, err := l.client.TTL(ctx, rlKey).Result()
		if err != nil || ttl < 0 {
			ttl = window
		}
		return false, ttl, nil
	}
	return true, 0, nil
}
