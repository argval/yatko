package handlers

import (
	"log"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

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
