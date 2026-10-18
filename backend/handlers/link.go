package handlers

import (
	"log"
	"net/http"

	"github.com/argval/yatko/github"
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
	URL            string               `json:"url"`
	Filename       string               `json:"filename,omitempty"`
	Size           int64                `json:"size,omitempty"`
	Platform       string               `json:"platform"`
	Arch           string               `json:"arch"`
	Version        string               `json:"version,omitempty"`
	Prefer         string               `json:"prefer,omitempty"`
	Libc           string               `json:"libc,omitempty"`
	Confidence     string               `json:"confidence,omitempty"`
	Reasons        []string             `json:"reasons,omitempty"`
	Status         string               `json:"status"`
	PlatformSource string               `json:"platform_source,omitempty"`
	ArchSource     string               `json:"arch_source,omitempty"`
	Error          string               `json:"error,omitempty"`
	Alternatives   []releaseAlternative `json:"alternatives,omitempty"`
}

func linkResponse(release *github.Release, p assetPick) LinkResponse {
	resp := LinkResponse{
		Platform:       string(p.Platform),
		Arch:           string(p.Arch),
		Version:        release.TagName,
		Prefer:         p.Prefer,
		Libc:           string(p.Libc),
		Confidence:     string(p.Decision.Confidence),
		Reasons:        p.Decision.Reasons,
		Status:         p.Decision.Status,
		PlatformSource: string(p.PlatformSource),
		ArchSource:     string(p.ArchSource),
	}
	if p.Decision.ShouldAutoSelect() {
		resp.URL = p.Decision.Asset.BrowserDownloadURL
		resp.Filename = p.Decision.Asset.Name
		resp.Size = p.Decision.Asset.Size
		return resp
	}
	resp.URL = release.HTMLURL
	resp.Error = "no suitable asset found for platform"
	resp.Alternatives = releaseDecision(p.Decision).Alternatives
	return resp
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

	p := h.redirect.pick(c, owner, repo, release.Assets)
	h.redirect.startShadow(c, owner, repo, release, p)
	if !p.Decision.ShouldAutoSelect() {
		logPickerMiss(owner, repo, p, IsScriptUA(p.UA), release.Assets)
		c.JSON(http.StatusNotFound, linkResponse(release, p))
		return
	}
	logPickerShadow(owner, repo, p)
	c.JSON(http.StatusOK, linkResponse(release, p))
}
