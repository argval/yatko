package picker

import (
	"strings"
	"unicode"

	"github.com/argval/yatko/github"
)

type Platform string

const (
	Windows Platform = "windows"
	MacOS   Platform = "macos"
	Linux   Platform = "linux"
	Android Platform = "android"
	IOS     Platform = "ios"
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
// win64/win32 are arch signals used by many Windows builds (Godot, x16emu, …);
// intel/m1 cover macOS asset names that omit amd64/arm64 tokens.
var archKeywords = map[Arch][]string{
	AMD64: {"amd64", "x86_64", "x86-64", "x64", "win64", "intel"},
	ARM64: {"arm64", "aarch64", "m1", "m2", "m3", "m4"},
	ARM:   {"armv7", "armv6", "armhf", "arm-"},
	X86:   {"i386", "i686", "x86_32", "386", "win32"},
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

// ResolvePlatform returns the Platform to use for asset selection. An explicit
// platform query-param takes priority when recognised; otherwise DetectPlatform
// on the User-Agent is used. Unknown falls back to Windows so opaque desktop
// UAs still get a sensible default (historical /api/link behaviour).
func ResolvePlatform(platformParam, userAgent string) Platform {
	switch strings.ToLower(strings.TrimSpace(platformParam)) {
	case "windows":
		return Windows
	case "macos", "darwin", "mac":
		return MacOS
	case "linux":
		return Linux
	case "android":
		return Android
	case "ios":
		return IOS
	}
	if p := DetectPlatform(userAgent); p != Unknown {
		return p
	}
	return Windows
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
// Android must be checked before Linux (Android UAs contain "Linux"), and
// iPhone/iPad/iPod before macOS (iOS UAs contain "like Mac OS X").
func DetectPlatform(userAgent string) Platform {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "windows") || strings.Contains(ua, "win64") || strings.Contains(ua, "win32"):
		return Windows
	case strings.Contains(ua, "android"):
		return Android
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "ipod"):
		return IOS
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
	Windows: {".exe", ".msi", ".zip", ".jar"},
	MacOS:   {".dmg", ".pkg", ".zip", ".tar.gz", ".jar"},
	Linux:   {".AppImage", ".deb", ".rpm", ".tar.gz", ".tar.xz", ".zip", ".jar"},
	Android: {".apk", ".aab"},
	IOS:     {".ipa"},
}

// variantKeywords are filename tokens that mark secondary builds (profiling,
// debug symbols, CPU-feature fallbacks, alternate runtimes). Prefer the
// vanilla asset when both exist — e.g. bun-darwin-aarch64.zip over
// bun-darwin-aarch64-profile.zip, or Godot win64 over mono_win64.
var variantKeywords = []string{"profile", "debug", "symbols", "dbg", "baseline", "mono", "pdb", "pdbs"}

// nonNativeKeywords mark browser/VM targets that are never the right pick for
// a desktop or mobile download CTA (e.g. x16emu_wasm-*.zip).
var nonNativeKeywords = []string{"wasm", "wasi"}

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

// Libc is an optional C library / linkage preference for Linux-style assets.
type Libc string

const (
	LibcAny    Libc = ""
	LibcMusl   Libc = "musl"
	LibcGNU    Libc = "gnu"
	LibcStatic Libc = "static"
)

// PickOpts controls optional format and libc preferences for asset selection.
type PickOpts struct {
	Prefer string // normalized extension key without leading dot (e.g. "deb")
	Libc   Libc
}

// ResolvePrefer normalizes a ?prefer= query value to an extension key
// (no leading dot). Unrecognised values return "" (ignored).
func ResolvePrefer(param string) string {
	p := strings.ToLower(strings.TrimSpace(param))
	p = strings.TrimPrefix(p, ".")
	switch p {
	case "app-image":
		p = "appimage"
	case "tgz":
		p = "tar.gz"
	case "txz":
		p = "tar.xz"
	}
	switch p {
	case "exe", "msi", "zip", "jar", "dmg", "pkg", "appimage",
		"deb", "rpm", "tar.gz", "tar.xz", "apk", "aab", "ipa":
		return p
	default:
		return ""
	}
}

// ResolveLibc normalizes a ?libc= query value. Unrecognised values → LibcAny.
func ResolveLibc(param string) Libc {
	switch strings.ToLower(strings.TrimSpace(param)) {
	case "musl":
		return LibcMusl
	case "gnu", "glibc":
		return LibcGNU
	case "static":
		return LibcStatic
	default:
		return LibcAny
	}
}

// libcPenalty ranks filename libc/linkage markers. Lower is better.
// When want is LibcAny, prefer untagged names over musl/gnu/glibc tags.
// Explicit musl/gnu/static treats static as compatible with musl or gnu.
func libcPenalty(name string, want Libc) int {
	musl := hasBoundedKeyword(name, "musl")
	gnu := hasBoundedKeyword(name, "gnu") || hasBoundedKeyword(name, "glibc")
	static := hasBoundedKeyword(name, "static")
	tagged := musl || gnu

	switch want {
	case LibcMusl:
		if musl || static {
			return 0
		}
		if gnu {
			return 2
		}
		return 1
	case LibcGNU:
		if gnu || static {
			return 0
		}
		if musl {
			return 2
		}
		return 1
	case LibcStatic:
		if static {
			return 0
		}
		return 1
	default:
		if tagged {
			return 1
		}
		return 0
	}
}

// extRankFor returns the platform extension rank for name. When prefer matches
// the asset's extension, rank is -1 so it beats the default order.
func extRankFor(name string, exts []string, prefer string) (rank int, ok bool) {
	for i, ext := range exts {
		low := strings.ToLower(ext)
		if !strings.HasSuffix(name, low) {
			continue
		}
		key := strings.TrimPrefix(low, ".")
		if prefer != "" && prefer == key {
			return -1, true
		}
		return i, true
	}
	return 0, false
}

// PickAssetForArch selects the best matching release asset for the given platform and
// CPU architecture. When arch is UnknownArch, architecture is ignored.
func PickAssetForArch(assets []github.Asset, platform Platform, arch Arch) *github.Asset {
	return PickAssetForArchOpts(assets, platform, arch, PickOpts{})
}

// PickAssetForArchOpts is PickAssetForArch with optional prefer/libc overrides.
func PickAssetForArchOpts(assets []github.Asset, platform Platform, arch Arch, opts PickOpts) *github.Asset {
	if len(assets) == 0 {
		return nil
	}

	type scored struct {
		asset       github.Asset
		extRank     int  // lower = better extension match
		archHit     bool // true when the asset explicitly matches the requested arch
		platformHit bool // true when the asset explicitly names this platform
		family      int  // lower = closer CPU family to the requested arch
		bitWidth    int  // 0 = 64-bit, 1 = unspecified, 2 = 32-bit (lower better)
		libc        int  // lower = better libc/linkage match
		variant     int  // lower = fewer secondary-build markers (profile/debug/…)
	}

	exts, ok := platformExtensions[platform]
	if !ok {
		return nil
	}

	prefer := ResolvePrefer(opts.Prefer)
	libc := opts.Libc
	if s := string(libc); s != "" {
		libc = ResolveLibc(s)
	}

	var candidates []scored

	for _, asset := range assets {
		name := canonicalizeName(asset.Name)
		if isSource(name) {
			continue
		}
		if isNonNative(name) {
			continue
		}
		if mentionsOtherPlatform(name, platform) {
			continue
		}

		rank, matched := extRankFor(name, exts, prefer)
		if !matched {
			continue
		}
		archHit := arch != UnknownArch && mentionsArch(name, arch)
		candidates = append(candidates, scored{
			asset:       asset,
			extRank:     rank,
			archHit:     archHit,
			platformHit: mentionsPlatform(name, platform),
			family:      archFamilyPenalty(name, arch),
			bitWidth:    archBitWidth(name),
			libc:        libcPenalty(name, libc),
			variant:     variantPenalty(name),
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	// When we have arch context, prefer assets that explicitly match the
	// requested arch; otherwise drop assets that name a conflicting arch so
	// we don't hand a win32 build to a 64-bit host just because it listed first.
	// If every candidate conflicts (e.g. only amd64+arm64 when asking for 386),
	// keep them all and let archFamilyPenalty prefer the closer family.
	if arch != UnknownArch {
		var archMatches []scored
		for _, c := range candidates {
			if c.archHit {
				archMatches = append(archMatches, c)
			}
		}
		if len(archMatches) > 0 {
			candidates = archMatches
		} else {
			var compatible []scored
			for _, c := range candidates {
				if !mentionsOtherArch(canonicalizeName(c.asset.Name), arch) {
					compatible = append(compatible, c)
				}
			}
			if len(compatible) > 0 {
				candidates = compatible
			}
		}
	}

	// Prefer platform-tagged assets over neutral names when both match the
	// extension filter (avoids picking linuxX64.zip for a Windows CTA when the
	// OS token was glued onto the arch, before canonicalizeName split it).
	var platformMatches []scored
	for _, c := range candidates {
		if c.platformHit {
			platformMatches = append(platformMatches, c)
		}
	}
	if len(platformMatches) > 0 {
		candidates = platformMatches
	}

	// Prefer better extension, closer CPU family, 64-bit when arch is unknown,
	// libc match, then vanilla (non-profile/debug/pdb/mono) builds.
	best := candidates[0]
	for _, c := range candidates[1:] {
		if candidateBetter(
			c.extRank, c.family, c.bitWidth, c.libc, c.variant,
			best.extRank, best.family, best.bitWidth, best.libc, best.variant,
		) {
			best = c
		}
	}
	return &best.asset
}

// candidateBetter reports whether a candidate outranks the current best.
func candidateBetter(ext, family, bits, libc, variant, bestExt, bestFamily, bestBits, bestLibc, bestVariant int) bool {
	if ext != bestExt {
		return ext < bestExt
	}
	if family != bestFamily {
		return family < bestFamily
	}
	if bits != bestBits {
		return bits < bestBits
	}
	if libc != bestLibc {
		return libc < bestLibc
	}
	return variant < bestVariant
}

// archFamilyPenalty ranks how close name's arch is to want when an exact
// match is unavailable. Lower is better. Same-family 64-bit (386→amd64,
// arm→arm64) beats the opposite family, so Windows 386 hosts don't get an
// arm64 build just because it was listed first.
func archFamilyPenalty(name string, want Arch) int {
	if want == UnknownArch {
		return 0
	}
	if mentionsArch(name, want) {
		return 0
	}
	hasAMD64 := mentionsArch(name, AMD64)
	hasARM64 := mentionsArch(name, ARM64)
	hasX86 := mentionsArch(name, X86)
	hasARM := mentionsArch(name, ARM)
	switch want {
	case AMD64:
		switch {
		case hasX86:
			return 2
		case hasARM64:
			return 3
		case hasARM:
			return 4
		}
	case X86:
		switch {
		case hasAMD64:
			return 1
		case hasARM64:
			return 3
		case hasARM:
			return 4
		}
	case ARM64:
		switch {
		case hasARM:
			return 2
		case hasAMD64:
			return 3
		case hasX86:
			return 4
		}
	case ARM:
		switch {
		case hasARM64:
			return 1
		case hasAMD64:
			return 3
		case hasX86:
			return 4
		}
	}
	return 5
}

// canonicalizeName inserts separators before an Uppercase+digit run that
// follows a lowercase letter (winX64 → win-x64), then lowercases. The digit
// look-ahead avoids splitting normal PascalCase tokens like AppImage into
// app-image, which would break extension and keyword matching.
func canonicalizeName(name string) string {
	if name == "" {
		return ""
	}
	runes := []rune(name)
	var b strings.Builder
	b.Grow(len(name) + 4)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[i-1]) {
			if i+1 < len(runes) && unicode.IsDigit(runes[i+1]) {
				b.WriteByte('-')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// isNonNative reports browser/VM targets that should never win a native download.
func isNonNative(name string) bool {
	for _, kw := range nonNativeKeywords {
		if hasBoundedKeyword(name, kw) {
			return true
		}
	}
	return false
}

// archBitWidth ranks 64-bit above unspecified above 32-bit. Used when the
// client arch is unknown so we still prefer win64 over win32.
func archBitWidth(name string) int {
	if mentionsArch(name, AMD64) || mentionsArch(name, ARM64) {
		return 0
	}
	if mentionsArch(name, X86) || mentionsArch(name, ARM) {
		return 2
	}
	return 1
}

// mentionsOtherArch reports whether name explicitly names an arch other than want.
func mentionsOtherArch(name string, want Arch) bool {
	for arch := range archKeywords {
		if arch == want {
			continue
		}
		if mentionsArch(name, arch) {
			return true
		}
	}
	return false
}

// mentionsPlatform reports whether the filename explicitly references platform.
func mentionsPlatform(name string, platform Platform) bool {
	keywords, ok := platformKeywords[platform]
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

// ambiguousTarballExts are archive suffixes that are used both for platform
// binaries and for source distributions. Without an OS/arch token in the
// name, treat them as source (e.g. htop-3.5.2.tar.xz).
var ambiguousTarballExts = []string{".tar.gz", ".tar.xz", ".tgz", ".txz"}

func isAmbiguousTarball(name string) bool {
	for _, ext := range ambiguousTarballExts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// platformKeywords maps each platform to the substrings that identify it in
// asset filenames. Shared by mentionsAnyPlatform / mentionsOtherPlatform.
// Bare "win" / "mac" are safe with hasBoundedKeyword: "win" inside "darwin"
// fails the leading-letter boundary check (dar-WIN), which is what "win-"
// used to paper over before camelCase names like "winX64" needed a token
// that isn't hyphen-terminated.
var platformKeywords = map[Platform][]string{
	Windows: {"windows", "win32", "win64", "win-", "win"},
	MacOS:   {"macos", "darwin", "osx", "mac-", "mac"},
	Linux:   {"linux", "ubuntu", "debian", "fedora", "appimage"},
	Android: {"android", "apk"},
	IOS:     {"ios", "iphone", "ipad", "ipod"},
}

func mentionsAnyPlatform(name string) bool {
	for _, keywords := range platformKeywords {
		for _, kw := range keywords {
			if hasBoundedKeyword(name, kw) {
				return true
			}
		}
	}
	return false
}

func mentionsAnyArch(name string) bool {
	for _, keywords := range archKeywords {
		for _, kw := range keywords {
			if hasBoundedKeyword(name, kw) {
				return true
			}
		}
	}
	return false
}

// isSource returns true if the filename looks like a source archive.
func isSource(name string) bool {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "source") || strings.Contains(lower, "src") {
		return true
	}
	// Bare versioned tarballs with no OS/arch tokens are source dists.
	if isAmbiguousTarball(lower) && !mentionsAnyPlatform(lower) && !mentionsAnyArch(lower) {
		return true
	}
	return false
}

// mentionsOtherPlatform checks if a filename explicitly references a different platform.
func mentionsOtherPlatform(name string, current Platform) bool {
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
