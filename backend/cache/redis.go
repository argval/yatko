package cache

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// DefaultSoftTTL is how long a cached value is served without revalidating
// against GitHub. Override with CACHE_TTL_SECONDS.
const DefaultSoftTTL = 15 * time.Minute

// HardTTL is how long an entry (and its ETag) survives in Redis after it
// goes stale. Keeping it well beyond SoftTTL means revalidation can keep
// happening cheaply via conditional requests (which don't count against
// GitHub's rate limit) instead of falling back to a full, rate-limited fetch.
const HardTTL = 24 * time.Hour

type Cache struct {
	client  *redis.Client
	softTTL time.Duration
	hardTTL time.Duration
	sf      singleflight.Group
}

func New() *Cache {
	softTTL := DefaultSoftTTL
	if v := os.Getenv("CACHE_TTL_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			softTTL = time.Duration(secs) * time.Second
		} else {
			fmt.Fprintf(os.Stderr, "warning: invalid CACHE_TTL_SECONDS=%q, using default %s\n", v, DefaultSoftTTL)
		}
	}

	redisURL := os.Getenv("UPSTASH_REDIS_URL")
	if redisURL == "" {
		// Return a no-op cache for local dev without Redis
		return &Cache{softTTL: softTTL, hardTTL: HardTTL}
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: invalid UPSTASH_REDIS_URL, caching disabled: %v\n", err)
		return &Cache{softTTL: softTTL, hardTTL: HardTTL}
	}
	opt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	return &Cache{
		client:  redis.NewClient(opt),
		softTTL: softTTL,
		hardTTL: HardTTL,
	}
}

// ReleaseKey, ReleaseTagKey, ReleasesKey, ReadmeKey and DescriptionKey build
// the cache keys used by FetchCached for each resource type.
func ReleaseKey(owner, repo string) string {
	return fmt.Sprintf("release:%s/%s", owner, repo)
}

func ReleaseTagKey(owner, repo, tag string) string {
	return fmt.Sprintf("release:%s/%s@%s", owner, repo, tag)
}

func ReleasesKey(owner, repo string) string {
	return fmt.Sprintf("releases:%s/%s", owner, repo)
}

func ReadmeKey(owner, repo string) string {
	return fmt.Sprintf("readme:%s/%s", owner, repo)
}

func DescriptionKey(owner, repo string) string {
	return fmt.Sprintf("description:%s/%s", owner, repo)
}

// SearchKey builds the cache key for a normalized GitHub repo search query.
func SearchKey(query string) string {
	return fmt.Sprintf("search:%s", query)
}

// entry is what's actually persisted in Redis: the value, the ETag GitHub
// returned for it (used for conditional revalidation), and when it was last
// confirmed fresh.
type entry[T any] struct {
	Value    T         `json:"value"`
	ETag     string    `json:"etag"`
	CachedAt time.Time `json:"cached_at"`
}

// ConditionalFetch performs a conditional GET against GitHub. It should send
// `etag` as If-None-Match when non-empty. notModified=true means the origin
// returned 304 — the caller's existing cached value is still current.
type ConditionalFetch[T any] func(ctx context.Context, etag string) (value T, newETag string, notModified bool, err error)

// FetchCached returns the cached value for key, transparently revalidating
// with `fetch` once the soft TTL has elapsed.
//
//   - Fresh cache (within soft TTL): returned immediately, no network call.
//   - Stale cache: revalidated via a conditional request. A 304 response
//     extends freshness for free (doesn't count against GitHub's rate limit)
//     without needing a new value from the caller.
//   - No cache entry: a normal fetch populates the cache with the returned
//     value and ETag.
//   - Concurrent requests for the same key are coalesced via singleflight, so
//     a stampede of cache misses only triggers one origin fetch.
//   - If the origin fetch fails and a (possibly stale) cached value exists,
//     that value is served instead of propagating the error.
func FetchCached[T any](ctx context.Context, c *Cache, key string, fetch ConditionalFetch[T]) (T, error) {
	var zero T

	existing, err := getEntry[T](ctx, c, key)
	if err != nil {
		log.Printf("cache read error (%s): %v", key, err)
	}

	if existing != nil && time.Since(existing.CachedAt) < c.softTTL {
		return existing.Value, nil
	}

	etag := ""
	if existing != nil {
		etag = existing.ETag
	}

	result, err, _ := c.sf.Do(key, func() (interface{}, error) {
		value, newETag, notModified, ferr := fetch(ctx, etag)
		if ferr != nil {
			return nil, ferr
		}

		next := entry[T]{CachedAt: time.Now()}
		if notModified {
			if existing == nil {
				return nil, fmt.Errorf("origin reported not-modified but no cached value exists for %s", key)
			}
			next.Value = existing.Value
			next.ETag = existing.ETag
		} else {
			next.Value = value
			next.ETag = newETag
		}

		if setErr := setEntry(ctx, c, key, next); setErr != nil {
			log.Printf("cache write error (%s): %v", key, setErr)
		}

		return next.Value, nil
	})

	if err != nil {
		if existing != nil {
			log.Printf("origin fetch failed for %s, serving stale cache: %v", key, err)
			return existing.Value, nil
		}
		return zero, err
	}

	return result.(T), nil
}

// Allow reports whether a request identified by key is within limit for the
// given fixed window, incrementing key's counter as a side effect. Always
// allows (fails open) when Redis is unconfigured or unreachable, matching
// the rest of Cache's no-op-without-Redis behavior - a Redis hiccup should
// throttle nothing, not 500 the API.
func (c *Cache) Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error) {
	if c.client == nil {
		return true, 0, nil
	}

	rlKey := "ratelimit:" + key
	pipe := c.client.Pipeline()
	incr := pipe.Incr(ctx, rlKey)
	pipe.ExpireNX(ctx, rlKey, window) // only takes effect on the first request in a window
	if _, err := pipe.Exec(ctx); err != nil {
		return true, 0, err
	}

	if incr.Val() > int64(limit) {
		ttl, err := c.client.TTL(ctx, rlKey).Result()
		if err != nil || ttl < 0 {
			ttl = window
		}
		return false, ttl, nil
	}
	return true, 0, nil
}

// Invalidate deletes the cached entry for key, if any. No-op if Redis is
// unconfigured. The next FetchCached call for key is a full cache miss (no
// ETag to revalidate against) and re-fetches from origin.
func (c *Cache) Invalidate(ctx context.Context, key string) error {
	if c.client == nil {
		return nil
	}
	return c.client.Del(ctx, key).Err()
}

func getEntry[T any](ctx context.Context, c *Cache, key string) (*entry[T], error) {
	if c.client == nil {
		return nil, nil
	}

	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var e entry[T]
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func setEntry[T any](ctx context.Context, c *Cache, key string, e entry[T]) error {
	if c.client == nil {
		return nil
	}

	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, c.hardTTL).Err()
}
