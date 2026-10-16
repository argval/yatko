package handlers

import (
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

// IsScriptUA reports whether ua looks like curl/wget/CI/AI HTTP clients rather
// than a browser. Empty UA is treated as a script (common for some tools).
// Anything without the browser "Mozilla/" token is a script: curl, wget,
// Go's default client, python-requests, GitHub Actions, coding agents, etc.
func IsScriptUA(ua string) bool {
	if strings.TrimSpace(ua) == "" {
		return true
	}
	return !strings.Contains(strings.ToLower(ua), "mozilla/")
}

// assetPick is the shared /dl and /api/link decision: resolve visitor
// platform/arch from query + UA, then run the authoritative picker.
type assetPick struct {
	UA             string
	Prefer         string
	Platform       picker.Platform
	Arch           picker.Arch
	Libc           picker.Libc
	PlatformSource picker.ResolutionSource
	ArchSource     picker.ResolutionSource
	Decision       picker.AssetDecision
}

func pickReleaseAsset(c *gin.Context, assets []github.Asset) assetPick {
	// A page can pin its displayed filename. Resolve it only from this release
	// and still apply compatibility checks to the latest observations.
	if name := c.Query("asset"); name != "" {
		var pinned []github.Asset
		for _, a := range assets {
			if a.Name == name {
				pinned = append(pinned, a)
			}
		}
		assets = pinned
	}
	ua := c.GetHeader("User-Agent")
	visitor := picker.ResolveVisitor(c.Query("platform"), c.Query("arch"), ua, IsScriptUA(ua))
	prefer := picker.ResolvePrefer(c.Query("prefer"))
	libc := picker.ResolveLibc(c.Query("libc"))
	return assetPick{
		UA:             ua,
		Prefer:         prefer,
		Platform:       visitor.Platform,
		Arch:           visitor.Arch,
		Libc:           libc,
		PlatformSource: visitor.PlatformSource,
		ArchSource:     visitor.ArchSource,
		Decision: picker.DecideAsset(assets, visitor.Platform, visitor.Arch, picker.PickOpts{
			Prefer:    prefer,
			Libc:      libc,
			UserAgent: ua,
		}),
	}
}

func logPickerShadow(owner, repo string, p assetPick) {
	d := p.Decision
	if d.Asset == nil || d.Confidence == picker.ConfidenceHigh {
		return
	}
	log.Printf(
		"picker_shadow owner=%s repo=%s platform=%s arch=%s confidence=%s file=%q unknown=%q reasons=%q",
		owner, repo, p.Platform, p.Arch, d.Confidence, d.Asset.Name,
		picker.UnknownTokens(d.Asset.Name), d.Reasons,
	)
}

const maxMissAssetNames = 20

func logPickerMiss(
	owner, repo string,
	p assetPick,
	script bool,
	assets []github.Asset,
) {
	n := len(assets)
	names := make([]string, 0, maxMissAssetNames)
	for i, a := range assets {
		if i >= maxMissAssetNames {
			break
		}
		names = append(names, a.Name)
	}
	guess := ""
	if p.Decision.Asset != nil {
		guess = p.Decision.Asset.Name
	}
	unknown := pickerUnknownTokens(assets, p.Decision.Asset)
	log.Printf(
		"picker_miss owner=%s repo=%s platform=%s arch=%s prefer=%s libc=%s script=%v confidence=%s guess=%q reasons=%q unknown=%q assets=%d names=%q",
		owner, repo, p.Platform, p.Arch, p.Prefer, p.Libc, script, p.Decision.Confidence, guess, p.Decision.Reasons, unknown, n, names,
	)
}

// respondDownloadMiss handles /dl when no asset matched or confidence is too
// low to auto-select: scripts get 404 JSON (including how the platform was
// chosen); browsers keep the historical redirect to the GitHub release page.
func respondDownloadMiss(
	c *gin.Context,
	owner, repo string,
	release *github.Release,
	p assetPick,
) {
	script := IsScriptUA(c.GetHeader("User-Agent"))
	logPickerMiss(owner, repo, p, script, release.Assets)
	if script {
		c.JSON(http.StatusNotFound, gin.H{
			"error":           "no suitable asset found for platform",
			"platform":        string(p.Platform),
			"arch":            string(p.Arch),
			"url":             release.HTMLURL,
			"platform_source": string(p.PlatformSource),
			"arch_source":     string(p.ArchSource),
		})
		return
	}
	c.Redirect(http.StatusFound, release.HTMLURL)
}

func pickerUnknownTokens(assets []github.Asset, selected *github.Asset) []string {
	names := make([]string, 0, maxMissAssetNames)
	if selected != nil {
		names = append(names, selected.Name)
	} else {
		for i, a := range assets {
			if i >= maxMissAssetNames {
				break
			}
			names = append(names, a.Name)
		}
	}
	counts := picker.HarvestUnknownTokens(names)
	out := make([]string, 0, len(counts))
	for tok := range counts {
		out = append(out, tok)
	}
	sort.Strings(out)
	return out
}
