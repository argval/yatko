package picker

import (
	"strings"

	"github.com/argval/yatko/github"
)

type Platform string

const (
	Windows Platform = "windows"
	MacOS   Platform = "macos"
	Linux   Platform = "linux"
	Unknown Platform = "unknown"
)

type Arch string

const (
	AMD64   Arch = "amd64"   // x86-64
	ARM64   Arch = "arm64"   // aarch64
	ARM     Arch = "arm"     // 32-bit ARM (armv7, armv6)
	X86     Arch = "386"     // 32-bit x86
	UnknownArch Arch = ""
)

// archKeywords maps each Arch to the substrings that identify it in asset filenames.
var archKeywords = map[Arch][]string{
	AMD64: {"amd64", "x86_64", "x86-64", "x64"},
	ARM64: {"arm64", "aarch64"},
	ARM:   {"armv7", "armv6", "armhf", "arm-"},
	X86:   {"i386", "i686", "x86_32", "386"},
}

// ResolveArch returns the Arch to use for asset selection. The explicit arch
// query-param (e.g. "amd64", "arm64") takes priority; if empty or unrecognised,
// the function falls back to DetectArch on the User-Agent string.
func ResolveArch(archParam, userAgent string) Arch {
	switch strings.ToLower(strings.TrimSpace(archParam)) {
	case "amd64", "x86_64":
		return AMD64
	case "arm64", "aarch64":
		return ARM64
	case "arm", "armv7", "armhf":
		return ARM
	case "386", "x86", "i386":
		return X86
	}
	return DetectArch(userAgent)
}

// DetectArch attempts to derive the CPU architecture from a User-Agent string.
// Returns UnknownArch when the UA doesn't carry enough signal.
func DetectArch(userAgent string) Arch {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "arm64") || strings.Contains(ua, "aarch64"):
		return ARM64
	case strings.Contains(ua, "armv7") || strings.Contains(ua, "armv6") || strings.Contains(ua, "armhf"):
		return ARM
	case strings.Contains(ua, "x86_64") || strings.Contains(ua, "amd64") || strings.Contains(ua, "win64"):
		return AMD64
	case strings.Contains(ua, "i386") || strings.Contains(ua, "i686") || strings.Contains(ua, "wow64"):
		return X86
	default:
		return UnknownArch
	}
}

// DetectPlatform parses a User-Agent string to determine the client's OS.
func DetectPlatform(userAgent string) Platform {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "windows") || strings.Contains(ua, "win64") || strings.Contains(ua, "win32"):
		return Windows
	case strings.Contains(ua, "macintosh") || strings.Contains(ua, "mac os") || strings.Contains(ua, "darwin"):
		return MacOS
	case strings.Contains(ua, "linux") || strings.Contains(ua, "ubuntu") || strings.Contains(ua, "fedora") || strings.Contains(ua, "debian"):
		return Linux
	default:
		return Unknown
	}
}

// platformExtensions maps each platform to its preferred file extensions, in priority order.
var platformExtensions = map[Platform][]string{
	Windows: {".exe", ".msi", ".zip"},
	MacOS:   {".dmg", ".pkg", ".zip", ".tar.gz"},
	Linux:   {".AppImage", ".deb", ".rpm", ".tar.gz", ".tar.xz", ".zip"},
}

// variantKeywords are filename tokens that mark secondary builds (profiling,
// debug symbols, CPU-feature fallbacks). Prefer the vanilla asset when both
// exist — e.g. bun-darwin-aarch64.zip over bun-darwin-aarch64-profile.zip.
var variantKeywords = []string{"profile", "debug", "symbols", "dbg", "baseline"}

// variantPenalty counts how many secondary-build markers appear in name.
// Lower is better; 0 means a vanilla release asset.
func variantPenalty(name string) int {
	penalty := 0
	for _, kw := range variantKeywords {
		if hasBoundedKeyword(name, kw) {
			penalty++
		}
	}
	return penalty
}

// PickAssetForArch selects the best matching release asset for the given platform and
// CPU architecture. When arch is UnknownArch, architecture is ignored and the
// function behaves identically to PickAsset.
func PickAssetForArch(assets []github.Asset, platform Platform, arch Arch) *github.Asset {
	if len(assets) == 0 {
		return nil
	}

	type scored struct {
		asset    github.Asset
		extRank  int  // lower = better extension match
		archHit  bool // true when the asset explicitly matches the requested arch
		variant  int  // lower = fewer secondary-build markers (profile/debug/…)
	}

	exts, ok := platformExtensions[platform]
	if !ok {
		return nil
	}

	var candidates []scored

	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if isSource(name) {
			continue
		}
		if mentionsOtherPlatform(name, platform) {
			continue
		}

		for rank, ext := range exts {
			if strings.HasSuffix(name, strings.ToLower(ext)) {
				archHit := arch != UnknownArch && mentionsArch(name, arch)
				candidates = append(candidates, scored{
					asset:   asset,
					extRank: rank,
					archHit: archHit,
					variant: variantPenalty(name),
				})
				break
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// When we have arch context, prefer assets that explicitly mention the
	// requested arch; break ties by extension rank.
	if arch != UnknownArch {
		// Check if any candidate explicitly matches the arch. If so, filter to
		// only those; otherwise fall through to extension-rank ordering so we
		// still return something useful.
		var archMatches []scored
		for _, c := range candidates {
			if c.archHit {
				archMatches = append(archMatches, c)
			}
		}
		if len(archMatches) > 0 {
			candidates = archMatches
		}
	}

	// Prefer better extension, then vanilla (non-profile/debug/baseline) builds.
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.extRank < best.extRank || (c.extRank == best.extRank && c.variant < best.variant) {
			best = c
		}
	}
	return &best.asset
}

// mentionsArch returns true if the asset filename explicitly references the given arch.
func mentionsArch(name string, arch Arch) bool {
	keywords, ok := archKeywords[arch]
	if !ok {
		return false
	}
	for _, kw := range keywords {
		if hasBoundedKeyword(name, kw) {
			return true
		}
	}
	return false
}

// hasBoundedKeyword reports whether kw occurs in name as a standalone token,
// i.e. not glued onto an adjacent letter. Keywords like "win-" are meant to
// match "app-win-x64.zip", but a naive strings.Contains also matches "win-"
// inside "darwin-arm64" - the standard macOS release-asset naming convention -
// which silently excluded every darwin asset from platform/arch matching.
// A side is only checked when the keyword doesn't already end/start with its
// own delimiter (e.g. "win-" already asserts its right edge via the hyphen).
func hasBoundedKeyword(name, kw string) bool {
	if kw == "" {
		return false
	}
	kwStartsWithLetter := isLower(kw[0])
	kwEndsWithLetter := isLower(kw[len(kw)-1])
	start := 0
	for {
		rel := strings.Index(name[start:], kw)
		if rel == -1 {
			return false
		}
		idx := start + rel
		beforeOK := !kwStartsWithLetter || idx == 0 || !isLower(name[idx-1])
		afterIdx := idx + len(kw)
		afterOK := !kwEndsWithLetter || afterIdx == len(name) || !isLower(name[afterIdx])
		if beforeOK && afterOK {
			return true
		}
		start = idx + 1
	}
}

func isLower(b byte) bool {
	return b >= 'a' && b <= 'z'
}

// isSource returns true if the filename looks like a source archive.
func isSource(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "source") || strings.Contains(lower, "src")
}

// mentionsOtherPlatform checks if a filename explicitly references a different platform.
func mentionsOtherPlatform(name string, current Platform) bool {
	platformKeywords := map[Platform][]string{
		Windows: {"windows", "win32", "win64", "win-"},
		MacOS:   {"macos", "darwin", "osx", "mac-", "apple"},
		Linux:   {"linux", "ubuntu", "debian", "fedora", "appimage"},
	}

	for p, keywords := range platformKeywords {
		if p == current {
			continue
		}
		for _, kw := range keywords {
			if hasBoundedKeyword(name, kw) {
				return true
			}
		}
	}
	return false
}
