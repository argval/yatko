package handlers

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/yoink/cache"
	"github.com/yourusername/yoink/github"
	"github.com/yourusername/yoink/picker"
)

type RedirectHandler struct {
	gh    *github.Client
	cache *cache.Cache
}

func NewRedirectHandler(gh *github.Client, c *cache.Cache) *RedirectHandler {
	return &RedirectHandler{gh: gh, cache: c}
}

func (h *RedirectHandler) Handle(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")

	release, err := h.getRelease(c, owner, repo)
	if err != nil {
		log.Printf("error fetching release for %s/%s: %v", owner, repo, err)
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

// HandleVersioned handles /dl/:owner/:repo/:version — download a specific release tag.
func (h *RedirectHandler) HandleVersioned(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")
	version := c.Param("version")

	release, err := h.getReleaseByTag(c, owner, repo, version)
	if err != nil {
		log.Printf("error fetching release %s for %s/%s: %v", version, owner, repo, err)
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

// getRelease returns the latest release for owner/repo, transparently caching
// and revalidating via conditional GitHub requests (see cache.FetchCached).
func (h *RedirectHandler) getRelease(c *gin.Context, owner, repo string) (*github.Release, error) {
	key := cache.ReleaseKey(owner, repo)
	return cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (*github.Release, string, bool, error) {
		return h.gh.GetLatestRelease(ctx, owner, repo, etag)
	})
}

func (h *RedirectHandler) getReleaseByTag(c *gin.Context, owner, repo, tag string) (*github.Release, error) {
	key := cache.ReleaseTagKey(owner, repo, tag)
	return cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (*github.Release, string, bool, error) {
		return h.gh.GetReleaseByTag(ctx, owner, repo, tag, etag)
	})
}
