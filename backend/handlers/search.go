package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/gin-gonic/gin"
)

const (
	searchMinQueryLen = 2
	searchMaxQueryLen = 100
)

// SearchHandler serves GET /api/search?q= — GitHub repo autocomplete for the homepage.
type SearchHandler struct {
	gh    *github.Client
	cache *cache.Cache
}

func NewSearchHandler(gh *github.Client, c *cache.Cache) *SearchHandler {
	return &SearchHandler{gh: gh, cache: c}
}

func (h *SearchHandler) Handle(c *gin.Context) {
	q := normalizeSearchQuery(c.Query("q"))
	if utf8.RuneCountInString(q) < searchMinQueryLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query must be at least 2 characters"})
		return
	}
	if utf8.RuneCountInString(q) > searchMaxQueryLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query too long"})
		return
	}

	ctx := c.Request.Context()
	key := cache.SearchKey(q)

	// No exact entry yet: try a shorter cached prefix so typing past a warm
	// query (e.g. "cli" → "clip") doesn't wait on GitHub Search.
	if _, ok := cache.GetCached[[]github.SearchRepo](ctx, h.cache, key); !ok {
		if items, ok := h.prefixFallback(ctx, q); ok {
			go h.warmSearch(q)
			c.JSON(http.StatusOK, gin.H{"items": items})
			return
		}
	}

	items, err := cache.FetchCachedWithTTL(ctx, h.cache, key, cache.SearchSoftTTL, func(ctx context.Context, etag string) ([]github.SearchRepo, string, bool, error) {
		return h.gh.SearchRepositories(ctx, q, etag)
	})
	if err != nil {
		log.Printf("search: error for %q: %v", q, err)
		c.JSON(httpStatusFromError(err), gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *SearchHandler) warmSearch(q string) {
	key := cache.SearchKey(q)
	_, _ = cache.FetchCachedWithTTL(context.Background(), h.cache, key, cache.SearchSoftTTL, func(ctx context.Context, etag string) ([]github.SearchRepo, string, bool, error) {
		return h.gh.SearchRepositories(ctx, q, etag)
	})
}

func (h *SearchHandler) prefixFallback(ctx context.Context, q string) ([]github.SearchRepo, bool) {
	runes := []rune(q)
	for n := len(runes) - 1; n >= searchMinQueryLen; n-- {
		prefix := string(runes[:n])
		items, ok := cache.GetCached[[]github.SearchRepo](ctx, h.cache, cache.SearchKey(prefix))
		if !ok {
			continue
		}
		filtered := filterSearchItems(items, q)
		if len(filtered) > 0 {
			return filtered, true
		}
	}
	return nil, false
}

func filterSearchItems(items []github.SearchRepo, q string) []github.SearchRepo {
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

// normalizeSearchQuery trims whitespace and lowercases for stable cache keys.
// GitHub search is case-insensitive, so this does not change results.
func normalizeSearchQuery(q string) string {
	return strings.ToLower(strings.TrimSpace(q))
}
