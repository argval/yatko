package handlers

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type BadgeHandler struct {
	redirect *RedirectHandler // reuse its getRelease method
}

func NewBadgeHandler(r *RedirectHandler) *BadgeHandler {
	return &BadgeHandler{redirect: r}
}

func (h *BadgeHandler) Handle(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")

	release, err := h.redirect.getRelease(c, owner, repo, "")
	if err != nil {
		log.Printf("badge: error fetching release for %s/%s: %v", owner, repo, err)
		c.Data(http.StatusNotFound, "image/svg+xml", renderBadge("version", "unknown", "#e05d44"))
		return
	}

	version := release.TagName
	c.Data(http.StatusOK, "image/svg+xml", renderBadge("version", version, "#007ec6"))
}

func renderBadge(label, value, color string) []byte {
	labelWidth := len(label)*7 + 10
	valueWidth := len(value)*7 + 10
	totalWidth := labelWidth + valueWidth

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20">
  <linearGradient id="s" x2="0" y2="100%%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="r"><rect width="%d" height="20" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)">
    <rect width="%d" height="20" fill="#555"/>
    <rect x="%d" width="%d" height="20" fill="%s"/>
    <rect width="%d" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
  </g>
</svg>`,
		totalWidth,
		totalWidth,
		labelWidth, labelWidth, valueWidth, color,
		totalWidth,
		labelWidth/2, label,
		labelWidth/2, label,
		labelWidth+valueWidth/2, value,
		labelWidth+valueWidth/2, value,
	)
	return []byte(svg)
}
