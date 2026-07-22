// Package search owns homepage repo autocomplete: cache lookup, prefix
// reuse while the user types, and background warming. Handlers stay wire-only.
package search

import (
	"context"
	"strings"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
)

const (
	// MinQueryLen is the shortest query we will look up.
	MinQueryLen = 2
	// MaxQueryLen rejects oversized autocomplete queries.
	MaxQueryLen = 100
)

// Autocomplete looks up GitHub repos for a typing prefix, preferring warm
// cache entries (including shorter prefixes) over a cold Search API call.
type Autocomplete struct {
	gh    *github.Client
	cache *cache.Cache
}

func NewAutocomplete(gh *github.Client, c *cache.Cache) *Autocomplete {
	return &Autocomplete{gh: gh, cache: c}
}

// Suggest returns repos matching q. On a cold exact key it may reuse a shorter
// cached prefix (filtered client-side) and warm the exact key in the background.
func (a *Autocomplete) Suggest(ctx context.Context, q string) ([]github.SearchRepo, error) {
	key := cache.SearchKey(q)

	if _, ok := cache.GetCached[[]github.SearchRepo](ctx, a.cache, key); !ok {
		if items, ok := a.prefixFallback(ctx, q); ok {
			go a.warm(q)
			return items, nil
		}
	}

	return cache.FetchCachedWithTTL(ctx, a.cache, key, cache.SearchSoftTTL, func(ctx context.Context, etag string) ([]github.SearchRepo, string, bool, error) {
		return a.gh.SearchRepositories(ctx, q, etag)
	})
}

func (a *Autocomplete) warm(q string) {
	key := cache.SearchKey(q)
	_, _ = cache.FetchCachedWithTTL(context.Background(), a.cache, key, cache.SearchSoftTTL, func(ctx context.Context, etag string) ([]github.SearchRepo, string, bool, error) {
		return a.gh.SearchRepositories(ctx, q, etag)
	})
}

func (a *Autocomplete) prefixFallback(ctx context.Context, q string) ([]github.SearchRepo, bool) {
	runes := []rune(q)
	for n := len(runes) - 1; n >= MinQueryLen; n-- {
		prefix := string(runes[:n])
		items, ok := cache.GetCached[[]github.SearchRepo](ctx, a.cache, cache.SearchKey(prefix))
		if !ok {
			continue
		}
		filtered := FilterItems(items, q)
		if len(filtered) > 0 {
			return filtered, true
		}
	}
	return nil, false
}

// FilterItems keeps repos whose owner, name, or slug contain q (case-folded).
func FilterItems(items []github.SearchRepo, q string) []github.SearchRepo {
	out := make([]github.SearchRepo, 0, len(items))
	for _, item := range items {
		slug := strings.ToLower(item.Owner + "/" + item.Repo)
		if strings.Contains(slug, q) ||
			strings.Contains(strings.ToLower(item.Repo), q) ||
			strings.Contains(strings.ToLower(item.Owner), q) {
			out = append(out, item)
		}
	}
	return out
}

// NormalizeQuery trims and lowercases for stable cache keys. GitHub search is
// case-insensitive, so this does not change result semantics.
func NormalizeQuery(q string) string {
	return strings.ToLower(strings.TrimSpace(q))
}
