package handlers

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/argval/yoink/cache"
	"github.com/argval/yoink/github"
	"github.com/argval/yoink/picker"
)

type RedirectHandler struct {
	gh    *github.Client
	cache *cache.Cache
}

func NewRedirectHandler(gh *github.Client, c *cache.Cache) *RedirectHandler {
	return &RedirectHandler{gh: gh, cache: c}
}

func (h *RedirectHandler) Handle(c *gin.Context) {
	h.handle(c, c.Param("owner"), c.Param("repo"), "")
}

// HandleVersioned handles /dl/:owner/:repo/:version — download a specific release tag.
func (h *RedirectHandler) HandleVersioned(c *gin.Context) {
	h.handle(c, c.Param("owner"), c.Param("repo"), c.Param("version"))
}

func (h *RedirectHandler) handle(c *gin.Context, owner, repo, version string) {
	release, err := h.getRelease(c, owner, repo, version)
	if err != nil {
		log.Printf("error fetching release %q for %s/%s: %v", version, owner, repo, err)
		c.JSON(httpStatusFromError(err), gin.H{"error": err.Error()})
		return
	}

	ua := c.GetHeader("User-Agent")
	platform := picker.DetectPlatform(ua)
	arch := picker.ResolveArch(c.Query("arch"), ua)
	asset := picker.PickAssetForArch(release.Assets, platform, arch)
	if asset == nil {
		c.Redirect(http.StatusFound, release.HTMLURL)
		return
	}

	c.Redirect(http.StatusFound, asset.BrowserDownloadURL)
}

// getRelease returns the release for owner/repo — the latest when version is
// empty, otherwise the given tag — transparently caching and revalidating via
// conditional GitHub requests (see cache.FetchCached).
func (h *RedirectHandler) getRelease(c *gin.Context, owner, repo, version string) (*github.Release, error) {
	if version == "" {
		key := cache.ReleaseKey(owner, repo)
		return cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (*github.Release, string, bool, error) {
			return h.gh.GetLatestRelease(ctx, owner, repo, etag)
		})
	}
	key := cache.ReleaseTagKey(owner, repo, version)
	return cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (*github.Release, string, bool, error) {
		return h.gh.GetReleaseByTag(ctx, owner, repo, version, etag)
	})
}
