package handlers

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/argval/yoink/cache"
	"github.com/argval/yoink/github"
)

// ReleasesHandler serves /api/releases/:owner/:repo — a lightweight list of
// recent releases (tag, name, date, prerelease flag) for use in version selectors.
type ReleasesHandler struct {
	gh    *github.Client
	cache *cache.Cache
}

func NewReleasesHandler(gh *github.Client, c *cache.Cache) *ReleasesHandler {
	return &ReleasesHandler{gh: gh, cache: c}
}

func (h *ReleasesHandler) Handle(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")

	key := cache.ReleasesKey(owner, repo)
	releases, err := cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) ([]github.ReleaseSummary, string, bool, error) {
		return h.gh.GetReleases(ctx, owner, repo, etag)
	})
	if err != nil {
		log.Printf("releases: error fetching for %s/%s: %v", owner, repo, err)
		c.JSON(httpStatusFromError(err), gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, releases)
}
