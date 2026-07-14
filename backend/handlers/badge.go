package handlers

import (
	"log"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

type BadgeHandler struct {
	redirect *RedirectHandler // reuse its getRelease method
}

func NewBadgeHandler(r *RedirectHandler) *BadgeHandler {
	return &BadgeHandler{redirect: r}
}

// Handle redirects to a shields.io static badge - shields.io already renders
// this exact SVG format, so there's no reason to hand-roll it here.
func (h *BadgeHandler) Handle(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")

	version, color := "unknown", "e05d44"
	if release, err := h.redirect.getRelease(c, owner, repo, ""); err != nil {
		log.Printf("badge: error fetching release for %s/%s: %v", owner, repo, err)
	} else {
		version, color = release.TagName, "007ec6"
	}

	badgeURL := "https://img.shields.io/static/v1?" + url.Values{
		"label":   {"version"},
		"message": {version},
		"color":   {color},
	}.Encode()
	c.Redirect(http.StatusFound, badgeURL)
}
