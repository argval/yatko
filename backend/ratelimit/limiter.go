// Package ratelimit provides a per-key fixed-window HTTP throttle.
// It is intentionally separate from cache.Cache — FetchCached and Allow are
// different seams that only share Redis as a transport.
package ratelimit

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// localMaxKeys caps the in-process fallback map so a Redis outage under a
// flood of distinct keys cannot unbounded-grow backend memory.
const localMaxKeys = 10_000

// Limiter enforces a fixed-window request budget keyed by an opaque string
// (typically a client IP). Without a Redis URL env it always allows (local
// dev). When Redis is configured but unreachable, it falls back to a
// process-local fixed window instead of failing open.
type Limiter struct {
	client *redis.Client

	localMu sync.Mutex
	local   map[string]localWindow
}

type localWindow struct {
	count     int64
	windowEnd time.Time
}

// New builds a Limiter from REDIS_URL / KV_URL / UPSTASH_REDIS_URL.
// Without a usable URL the returned Limiter always allows.
func New() *Limiter {
	redisURL := firstEnv("REDIS_URL", "KV_URL", "UPSTASH_REDIS_URL")
	if redisURL == "" {
		return &Limiter{local: make(map[string]localWindow)}
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: invalid Redis URL, rate limiting disabled: %v\n", err)
		return &Limiter{local: make(map[string]localWindow)}
	}
	opt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &Limiter{
		client: redis.NewClient(opt),
		local:  make(map[string]localWindow),
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// Allow reports whether a request identified by key is within limit for the
// given fixed window, incrementing key's counter as a side effect.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error) {
	if l == nil {
		return true, 0, nil
	}
	if l.client == nil {
		return true, 0, nil
	}

	rlKey := "ratelimit:" + key
	pipe := l.client.Pipeline()
	incr := pipe.Incr(ctx, rlKey)
	pipe.ExpireNX(ctx, rlKey, window) // only takes effect on the first request in a window
	if _, err := pipe.Exec(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warning: rate limit redis error, using local fallback: %v\n", err)
		allowed, retryAfter := l.allowLocal(key, limit, window)
		return allowed, retryAfter, nil
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

func (l *Limiter) allowLocal(key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration) {
	l.localMu.Lock()
	defer l.localMu.Unlock()

	if l.local == nil {
		l.local = make(map[string]localWindow)
	}

	now := time.Now()
	e, ok := l.local[key]
	if !ok || !now.Before(e.windowEnd) {
		if len(l.local) >= localMaxKeys {
			// Drop expired entries first; if still full, clear to bound memory.
			for k, w := range l.local {
				if !now.Before(w.windowEnd) {
					delete(l.local, k)
				}
			}
			if len(l.local) >= localMaxKeys {
				l.local = make(map[string]localWindow, localMaxKeys/4)
			}
		}
		l.local[key] = localWindow{count: 1, windowEnd: now.Add(window)}
		return true, 0
	}

	e.count++
	l.local[key] = e
	if e.count > int64(limit) {
		return false, e.windowEnd.Sub(now)
	}
	return true, 0
}
