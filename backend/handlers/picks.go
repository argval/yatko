package handlers

import (
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

// releasePick is a precomputed auto-select for one platform/arch pair, embedded
// in /api/release so the download button can show the filename without a
// second /api/link round trip. Click still goes through /dl (Go decides again
// with the live UA, including Linux deb/rpm).
type releasePick struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
}

// releasePickTargets covers the visitor combos the release page can detect.
// Empty arch covers UAs that never resolve an arch (rare after WebGL on Mac).
var releasePickTargets = []struct {
	platform picker.Platform
	arch     picker.Arch
}{
	{picker.MacOS, picker.ARM64},
	{picker.MacOS, picker.AMD64},
	{picker.MacOS, picker.UnknownArch},
	{picker.Windows, picker.AMD64},
	{picker.Windows, picker.ARM64},
	{picker.Windows, picker.X86},
	{picker.Windows, picker.UnknownArch},
	{picker.Linux, picker.AMD64},
	{picker.Linux, picker.ARM64},
	{picker.Linux, picker.ARM},
	{picker.Linux, picker.UnknownArch},
	{picker.Android, picker.ARM64},
	{picker.Android, picker.ARM},
	{picker.Android, picker.UnknownArch},
	{picker.IOS, picker.ARM64},
	{picker.IOS, picker.UnknownArch},
}

func releasePickKey(platform picker.Platform, arch picker.Arch) string {
	if arch == picker.UnknownArch {
		return string(platform)
	}
	return string(platform) + "/" + string(arch)
}

// buildReleasePicks runs DecideAsset for each common visitor combo. Only
// auto-select decisions are included; a missing key means abstain.
func buildReleasePicks(assets []github.Asset) map[string]releasePick {
	if len(assets) == 0 {
		return nil
	}
	out := make(map[string]releasePick, len(releasePickTargets))
	for _, t := range releasePickTargets {
		d := picker.DecideAsset(assets, t.platform, t.arch, picker.PickOpts{})
		if !d.ShouldAutoSelect() {
			continue
		}
		out[releasePickKey(t.platform, t.arch)] = releasePick{
			Filename: d.Asset.Name,
			URL:      d.Asset.BrowserDownloadURL,
			Size:     d.Asset.Size,
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
