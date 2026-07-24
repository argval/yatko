package handlers

import (
	"context"
	"log"
	"net/http"

	"github.com/argval/yatko/archive"
	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

type RedirectHandler struct {
	gh      *github.Client
	cache   *cache.Cache
	archive *archive.Service
}

func NewRedirectHandler(gh *github.Client, c *cache.Cache, arch *archive.Service) *RedirectHandler {
	return &RedirectHandler{gh: gh, cache: c, archive: arch}
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
		c.JSON(httpStatusFromError(err), gin.H{"error": publicErrorMessage(err)})
		return
	}

	ua := c.GetHeader("User-Agent")
	platform := picker.DetectPlatform(ua)
	arch := picker.ResolveArch(c.Query("arch"), ua)
	asset := picker.PickAssetForArch(release.Assets, platform, arch)
	if asset == nil {
		// Source-only mirrors (e.g. iOS bitchat) have no platform binary —
		// still hand the user the single mirrored asset instead of GitHub HTML.
		if fallback := soleDownloadable(release.Assets); fallback != nil {
			c.Redirect(http.StatusFound, fallback.BrowserDownloadURL)
			return
		}
		c.Redirect(http.StatusFound, release.HTMLURL)
		return
	}

	c.Redirect(http.StatusFound, asset.BrowserDownloadURL)
}

func soleDownloadable(assets []github.Asset) *github.Asset {
	var only *github.Asset
	for i := range assets {
		if assets[i].BrowserDownloadURL == "" {
			continue
		}
		if only != nil {
			return nil
		}
		only = &assets[i]
	}
	return only
}

// getRelease returns the release for owner/repo — the latest when version is
// empty, otherwise the given tag — transparently caching and revalidating via
// conditional GitHub requests (see cache.FetchCached). Archived repos overlay
// Blob mirror URLs (and fall back to the last synced manifest if GitHub fails).
func (h *RedirectHandler) getRelease(c *gin.Context, owner, repo, version string) (*github.Release, error) {
	release, err := h.fetchRelease(c.Request.Context(), owner, repo, version)
	if h.archive != nil {
		return h.archive.Resolve(c.Request.Context(), owner, repo, version, release, err)
	}
	return release, err
}

func (h *RedirectHandler) fetchRelease(ctx context.Context, owner, repo, version string) (*github.Release, error) {
	if version == "" {
		key := cache.ReleaseKey(owner, repo)
		return cache.FetchCached(ctx, h.cache, key, func(ctx context.Context, etag string) (*github.Release, string, bool, error) {
			return h.gh.GetLatestRelease(ctx, owner, repo, etag)
		})
	}
	key := cache.ReleaseTagKey(owner, repo, version)
	return cache.FetchCached(ctx, h.cache, key, func(ctx context.Context, etag string) (*github.Release, string, bool, error) {
		return h.gh.GetReleaseByTag(ctx, owner, repo, version, etag)
	})
}
