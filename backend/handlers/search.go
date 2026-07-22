package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
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

	key := cache.SearchKey(q)
	items, err := cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) ([]github.SearchRepo, string, bool, error) {
		return h.gh.SearchRepositories(ctx, q, etag)
	})
	if err != nil {
		log.Printf("search: error for %q: %v", q, err)
		c.JSON(httpStatusFromError(err), gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// normalizeSearchQuery trims whitespace and lowercases for stable cache keys.
// GitHub search is case-insensitive, so this does not change results.
func normalizeSearchQuery(q string) string {
	return strings.ToLower(strings.TrimSpace(q))
}
