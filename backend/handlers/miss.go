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

// IsScriptUA reports whether ua looks like curl/wget/CI HTTP clients rather
// than a browser. Empty UA is treated as a script (common for some tools).
func IsScriptUA(ua string) bool {
	if strings.TrimSpace(ua) == "" {
		return true
	}
	lower := strings.ToLower(ua)
	markers := []string{
		"curl/",
		"wget",
		"httpie",
		"go-http-client",
		"python-requests",
		"python-urllib",
		"axios/",
		"node-fetch",
		"http.rb",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// assetPick is the shared /dl and /api/link decision: resolve visitor
// platform/arch from query + UA, then run the authoritative picker.
type assetPick struct {
	UA       string
	Prefer   string
	Platform picker.Platform
	Arch     picker.Arch
	Libc     picker.Libc
	Decision picker.AssetDecision
}

func pickReleaseAsset(c *gin.Context, assets []github.Asset) assetPick {
	ua := c.GetHeader("User-Agent")
	platform := picker.ResolvePlatform(c.Query("platform"), ua)
	arch := picker.ResolveArch(c.Query("arch"), ua)
	prefer := picker.ResolvePrefer(c.Query("prefer"))
	libc := picker.ResolveLibc(c.Query("libc"))
	return assetPick{
		UA:       ua,
		Prefer:   prefer,
		Platform: platform,
		Arch:     arch,
		Libc:     libc,
		Decision: picker.DecideAsset(assets, platform, arch, picker.PickOpts{
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
// low to auto-select: scripts get 404 JSON; browsers keep the historical
// redirect to the GitHub release HTML page.
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
			"error":    "no suitable asset found for platform",
			"platform": string(p.Platform),
			"arch":     string(p.Arch),
			"url":      release.HTMLURL,
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
