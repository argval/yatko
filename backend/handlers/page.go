package handlers

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/yoink/cache"
	"github.com/yourusername/yoink/github"
)

type PageHandler struct {
	redirect *RedirectHandler
	gh       *github.Client
	cache    *cache.Cache
}

func NewPageHandler(r *RedirectHandler, gh *github.Client, c *cache.Cache) *PageHandler {
	return &PageHandler{redirect: r, gh: gh, cache: c}
}

func (h *PageHandler) Handle(c *gin.Context) {
	h.handle(c, c.Param("owner"), c.Param("repo"), "")
}

// HandleVersioned serves /api/release/:owner/:repo/:version — metadata for a specific tag.
func (h *PageHandler) HandleVersioned(c *gin.Context) {
	h.handle(c, c.Param("owner"), c.Param("repo"), c.Param("version"))
}

func (h *PageHandler) handle(c *gin.Context, owner, repo, version string) {
	release, err := h.redirect.getRelease(c, owner, repo, version)
	if err != nil {
		log.Printf("page: error fetching release %q for %s/%s: %v", version, owner, repo, err)
		c.JSON(httpStatusFromError(err), gin.H{"error": err.Error()})
		return
	}

	readme := h.getREADME(c, owner, repo)
	description := h.getDescription(c, owner, repo)

	c.JSON(http.StatusOK, gin.H{
		"owner":        owner,
		"repo":         repo,
		"description":  description,
		"tag_name":     release.TagName,
		"name":         release.Name,
		"body":         release.Body,
		"published_at": release.PublishedAt,
		"html_url":     release.HTMLURL,
		"prerelease":   release.Prerelease,
		"assets":       release.Assets,
		"readme":       readme,
	})
}

func (h *PageHandler) getDescription(c *gin.Context, owner, repo string) string {
	key := cache.DescriptionKey(owner, repo)
	desc, err := cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (string, string, bool, error) {
		repoMeta, newETag, notModified, err := h.gh.GetRepo(ctx, owner, repo, etag)
		if err != nil {
			return "", "", false, err
		}
		if notModified {
			return "", newETag, true, nil
		}
		return repoMeta.Description, newETag, false, nil
	})
	if err != nil {
		log.Printf("repo fetch error for %s/%s: %v", owner, repo, err)
		return ""
	}
	return desc
}

func (h *PageHandler) getREADME(c *gin.Context, owner, repo string) string {
	key := cache.ReadmeKey(owner, repo)
	content, err := cache.FetchCached(c.Request.Context(), h.cache, key, func(ctx context.Context, etag string) (string, string, bool, error) {
		return h.gh.GetREADME(ctx, owner, repo, etag)
	})
	if err != nil {
		log.Printf("readme fetch error for %s/%s: %v", owner, repo, err)
		return ""
	}
	return content
}
