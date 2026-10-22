package handlers

import (
	"log"
	"maps"
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
	ClientHints    map[string]string
	Product        string // repo description; only fetched when Jev decides
	Decision       picker.AssetDecision
}

// resolveAssetPick keeps request context separate from asset ranking.
func resolveAssetPick(c *gin.Context) assetPick {
	ua := c.GetHeader("User-Agent")
	visitor := picker.ResolveVisitor(c.Query("platform"), c.Query("arch"), ua, IsScriptUA(ua))
	if visitor.PlatformSource == picker.SourceQuery {
		visitor.PlatformSource = visitorSource(c.Query("platform_source"))
	}
	if visitor.ArchSource == picker.SourceQuery {
		visitor.ArchSource = visitorSource(c.Query("arch_source"))
	}
	return assetPick{
		UA: ua, Prefer: picker.ResolvePrefer(c.Query("prefer")),
		Platform: visitor.Platform, Arch: visitor.Arch, Libc: picker.ResolveLibc(c.Query("libc")),
		PlatformSource: visitor.PlatformSource, ArchSource: visitor.ArchSource,
		ClientHints: visitorClientHints(c),
	}
}

// visitorSource preserves browser provenance while manual overrides remain queries.
func visitorSource(value string) picker.ResolutionSource {
	switch value {
	case "client-hints", "browser":
		return picker.ResolutionSource(value)
	default:
		return picker.SourceQuery
	}
}

// visitorClientHints accepts only bounded device hints, including navigation-safe query copies.
func visitorClientHints(c *gin.Context) map[string]string {
	hints := map[string]string{}
	for _, field := range []struct{ key, query, header string }{
		{"architecture", "hint_architecture", "Sec-CH-UA-Arch"},
		{"bitness", "hint_bitness", "Sec-CH-UA-Bitness"},
		{"platform", "hint_platform", "Sec-CH-UA-Platform"},
		{"platform_version", "hint_platform_version", "Sec-CH-UA-Platform-Version"},
		{"model", "hint_model", "Sec-CH-UA-Model"},
		{"mobile", "hint_mobile", "Sec-CH-UA-Mobile"},
	} {
		value := c.Query(field.query)
		if value == "" {
			value = c.GetHeader(field.header)
		}
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 || strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			continue
		}
		hints[field.key] = value
	}
	return hints
}

// opts provides the same visitor evidence to primary and fallback decisions.
func (p assetPick) opts() picker.PickOpts {
	return picker.PickOpts{Prefer: p.Prefer, Libc: p.Libc, UserAgent: p.UA,
		PlatformSource: p.PlatformSource, ArchSource: p.ArchSource, ClientHints: maps.Clone(p.ClientHints),
		ProductDescription: p.Product}
}

// ruleDecision applies conservative ranking to a pinned file or the full release.
func ruleDecision(c *gin.Context, assets []github.Asset, p assetPick) picker.AssetDecision {
	if name := c.Query("asset"); name != "" {
		var pinned []github.Asset
		for _, asset := range assets {
			if asset.Name == name {
				pinned = append(pinned, asset)
			}
		}
		assets = pinned
	}
	return picker.DecideAsset(assets, p.Platform, p.Arch, p.opts())
}

func pickReleaseAsset(c *gin.Context, assets []github.Asset) assetPick {
	p := resolveAssetPick(c)
	p.Decision = ruleDecision(c, assets, p)
	return p
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
