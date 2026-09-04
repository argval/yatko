package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// LinkHandler serves /api/link/:owner/:repo[/:version], returning JSON with the
// resolved download URL rather than issuing a redirect. Useful for scripts and
// CI pipelines that need the URL without following redirects:
//
//	curl -s yatko.app/api/link/cli/cli | jq -r .url | xargs wget
//
// Query params (same as /dl):
//
//	?platform=windows|macos|linux|android|ios
//	?arch=amd64|arm64|arm|386
//	?prefer=deb|rpm|appimage|msi|dmg|exe|pkg|apk|tar.gz|…
//	?libc=musl|gnu|static
type LinkHandler struct {
	redirect *RedirectHandler
}

func NewLinkHandler(r *RedirectHandler) *LinkHandler {
	return &LinkHandler{redirect: r}
}

// LinkResponse is the JSON payload returned by the link endpoint.
type LinkResponse struct {
	URL        string   `json:"url"`
	Filename   string   `json:"filename"`
	Size       int64    `json:"size"`
	Platform   string   `json:"platform"`
	Arch       string   `json:"arch"`
	Version    string   `json:"version"`
	Prefer     string   `json:"prefer,omitempty"`
	Libc       string   `json:"libc,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
	Reasons    []string `json:"reasons,omitempty"`
}

func (h *LinkHandler) Handle(c *gin.Context) {
	h.handle(c, c.Param("owner"), c.Param("repo"), "")
}

// HandleVersioned serves /api/link/:owner/:repo/:version.
func (h *LinkHandler) HandleVersioned(c *gin.Context) {
	h.handle(c, c.Param("owner"), c.Param("repo"), c.Param("version"))
}

func (h *LinkHandler) handle(c *gin.Context, owner, repo, version string) {
	release, err := h.redirect.getRelease(c, owner, repo, version)
	if err != nil {
		log.Printf("link: error fetching release %q for %s/%s: %v", version, owner, repo, err)
		c.JSON(httpStatusFromError(err), gin.H{"error": publicErrorMessage(err)})
		return
	}

	p := pickReleaseAsset(c, release.Assets)
	if !p.Decision.ShouldAutoSelect() {
		logPickerMiss(owner, repo, p, IsScriptUA(p.UA), release.Assets)
		c.JSON(http.StatusNotFound, gin.H{
			"error":      "no suitable asset found for platform",
			"platform":   string(p.Platform),
			"arch":       string(p.Arch),
			"url":        release.HTMLURL,
			"confidence": string(p.Decision.Confidence),
			"reasons":    p.Decision.Reasons,
		})
		return
	}
	logPickerShadow(owner, repo, p)

	c.JSON(http.StatusOK, LinkResponse{
		URL:        p.Decision.Asset.BrowserDownloadURL,
		Filename:   p.Decision.Asset.Name,
		Size:       p.Decision.Asset.Size,
		Platform:   string(p.Platform),
		Arch:       string(p.Arch),
		Version:    release.TagName,
		Prefer:     p.Prefer,
		Libc:       string(p.Libc),
		Confidence: string(p.Decision.Confidence),
		Reasons:    p.Decision.Reasons,
	})
}
