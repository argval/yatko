package handlers

import (
	"log"
	"net/http"

	"github.com/argval/yatko/picker"
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

	ua := c.GetHeader("User-Agent")
	platform := picker.ResolvePlatform(c.Query("platform"), ua)
	arch := picker.ResolveArch(c.Query("arch"), ua)
	prefer := picker.ResolvePrefer(c.Query("prefer"))
	libc := picker.ResolveLibc(c.Query("libc"))
	decision := picker.DecideAsset(release.Assets, platform, arch, picker.PickOpts{
		Prefer:    prefer,
		Libc:      libc,
		UserAgent: ua,
	})
	if !decision.ShouldAutoSelect() {
		logPickerMiss(owner, repo, platform, arch, prefer, libc, IsScriptUA(ua), release.Assets, decision)
		c.JSON(http.StatusNotFound, gin.H{
			"error":      "no suitable asset found for platform",
			"platform":   string(platform),
			"arch":       string(arch),
			"url":        release.HTMLURL,
			"confidence": string(decision.Confidence),
			"reasons":    decision.Reasons,
		})
		return
	}

	c.JSON(http.StatusOK, LinkResponse{
		URL:        decision.Asset.BrowserDownloadURL,
		Filename:   decision.Asset.Name,
		Size:       decision.Asset.Size,
		Platform:   string(platform),
		Arch:       string(arch),
		Version:    release.TagName,
		Prefer:     prefer,
		Libc:       string(libc),
		Confidence: string(decision.Confidence),
		Reasons:    decision.Reasons,
	})
}
