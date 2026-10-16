package picker

import (
	"strings"
	"unicode"
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
	AMD64       Arch = "amd64" // x86-64
	ARM64       Arch = "arm64" // aarch64
	ARM         Arch = "arm"   // 32-bit ARM (armv7, armv6)
	X86         Arch = "386"   // 32-bit x86
	UnknownArch Arch = ""
)

// ResolutionSource says how platform/arch was chosen for a visitor.
type ResolutionSource string

const (
	SourceQuery   ResolutionSource = "query"
	SourceUA      ResolutionSource = "ua"
	SourceDefault ResolutionSource = "default"
)

// Visitor is the OS/arch used for /dl and /api/link, plus how each was chosen.
type Visitor struct {
	Platform       Platform
	Arch           Arch
	PlatformSource ResolutionSource
	ArchSource     ResolutionSource
}

// ResolveVisitor combines query overrides, User-Agent parsing, and defaults.
// An explicit platform/arch query-param always wins. Otherwise the UA is
// parsed (including curl/wget triples like x86_64-apple-darwin). Opaque
// browser UAs stay unknown; script/CLI clients (curl, wget, CI, AI agents)
// default to linux/amd64 so a bare /dl/owner/repo is useful.
func ResolveVisitor(platformParam, archParam, userAgent string, script bool) Visitor {
	var v Visitor

	switch strings.ToLower(strings.TrimSpace(platformParam)) {
	case "windows":
		v.Platform, v.PlatformSource = Windows, SourceQuery
	case "macos", "darwin", "mac":
		v.Platform, v.PlatformSource = MacOS, SourceQuery
	case "linux":
		v.Platform, v.PlatformSource = Linux, SourceQuery
	case "android":
		v.Platform, v.PlatformSource = Android, SourceQuery
	case "ios":
		v.Platform, v.PlatformSource = IOS, SourceQuery
	default:
		if p := DetectPlatform(userAgent); p != Unknown {
			v.Platform, v.PlatformSource = p, SourceUA
		} else if script {
			v.Platform, v.PlatformSource = Linux, SourceDefault
		} else {
			v.Platform, v.PlatformSource = Unknown, SourceUA
		}
	}

	switch strings.ToLower(strings.TrimSpace(archParam)) {
	case "unknown":
		v.Arch, v.ArchSource = UnknownArch, SourceQuery
	case "amd64", "x86_64":
		v.Arch, v.ArchSource = AMD64, SourceQuery
	case "arm64", "aarch64":
		v.Arch, v.ArchSource = ARM64, SourceQuery
	case "arm", "armv7", "armhf":
		v.Arch, v.ArchSource = ARM, SourceQuery
	case "386", "x86", "i386":
		v.Arch, v.ArchSource = X86, SourceQuery
	default:
		if a := DetectArch(userAgent); a != UnknownArch {
			v.Arch, v.ArchSource = a, SourceUA
		} else if script {
			v.Arch, v.ArchSource = AMD64, SourceDefault
		} else {
			v.Arch, v.ArchSource = UnknownArch, SourceUA
		}
	}
	return v
}

// ResolveArch returns the Arch to use for asset selection. The explicit arch
// query-param (e.g. "amd64", "arm64") takes priority; "unknown" stays empty so
// the release page can pin "CPU not specified". Otherwise DetectArch on the UA.
func ResolveArch(archParam, userAgent string) Arch {
	return ResolveVisitor("", archParam, userAgent, false).Arch
}

// ResolvePlatform returns the Platform to use for asset selection. An explicit
// platform query-param takes priority when recognised; otherwise DetectPlatform
// on the User-Agent is used. Opaque browser UAs remain unknown.
func ResolvePlatform(platformParam, userAgent string) Platform {
	return ResolveVisitor(platformParam, "", userAgent, false).Platform
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
	case strings.Contains(ua, "x86_64") || strings.Contains(ua, "amd64") || strings.Contains(ua, "win64") || strings.Contains(ua, "x64"):
		return AMD64
	case strings.Contains(ua, "i386") || strings.Contains(ua, "i686") || strings.Contains(ua, "wow64"):
		return X86
	default:
		return UnknownArch
	}
}

// DetectPlatform parses a User-Agent string to determine the client's OS.
// Android must be checked before Linux (Android UAs contain "Linux"), and
// iPhone/iPad/iPod before macOS (iOS UAs contain "like Mac OS X"). CLI triples
// such as x86_64-apple-darwin / x86_64-w64-mingw32 / x86_64-pc-linux-gnu are
// recognised so curl/wget builds that embed a target tuple still match.
func DetectPlatform(userAgent string) Platform {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "windows") || strings.Contains(ua, "win64") || strings.Contains(ua, "win32") ||
		strings.Contains(ua, "mingw") || strings.Contains(ua, "msys") || strings.Contains(ua, "cygwin"):
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
	Prefer    string // normalized extension key without leading dot (e.g. "deb")
	Libc      Libc
	UserAgent string // optional; Linux deb/rpm tiebreak when Prefer is empty
}

// linuxExtensionsForUA returns Linux extension priority. Fedora/RHEL/CentOS UAs
// swap deb and rpm; AppImage and archives stay above both. Explicit ?prefer=
// still wins via extRankFor prefer boost.
func linuxExtensionsForUA(userAgent string) []string {
	if ResolveLinuxPackagePrefer(userAgent) == "rpm" {
		return catalog.LinuxRPMExtensions
	}
	exts, _ := platformExts(Linux)
	return exts
}

// ResolveLinuxPackagePrefer returns a package-format hint from the User-Agent
// for deb-vs-rpm tiebreaks on Linux. Empty when the UA carries no distro signal.
func ResolveLinuxPackagePrefer(userAgent string) string {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "ubuntu") || strings.Contains(ua, "debian"):
		return "deb"
	case strings.Contains(ua, "fedora") || strings.Contains(ua, "rhel") || strings.Contains(ua, "centos"):
		return "rpm"
	default:
		return ""
	}
}

// ResolvePrefer normalizes a ?prefer= query value to an extension key
// (no leading dot). Unrecognised values return "" (ignored).
func ResolvePrefer(param string) string {
	p := strings.ToLower(strings.TrimSpace(param))
	p = strings.TrimPrefix(p, ".")
	if alias, ok := catalog.PreferAliases[p]; ok {
		p = alias
	}
	if _, ok := catalog.Formats[p]; ok {
		return p
	}
	return ""
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

// extRankFor returns the platform extension rank for name. When prefer matches
// the asset's extension, rank is -1 so it beats the default order.
func extRankFor(name string, exts []string, prefer string) (rank int, ok bool) {
	for i, ext := range exts {
		low := strings.ToLower(ext)
		if !strings.HasSuffix(name, low) {
			continue
		}
		key := strings.TrimPrefix(low, ".")
		if alias, ok := catalog.PreferAliases[key]; ok {
			key = alias
		}
		if prefer != "" && prefer == key {
			return -1, true
		}
		return i, true
	}
	return 0, false
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
	for _, kw := range catalog.NonNative {
		if hasBoundedKeyword(name, kw) {
			return true
		}
	}
	return false
}

type keywordSpan struct {
	start, end int
}

// hasBoundedKeyword reports whether kw occurs in name as a standalone token,
// i.e. not glued onto an adjacent letter. Keywords like "win-" are meant to
// match "app-win-x64.zip", but a naive strings.Contains also matches "win-"
// inside "darwin-arm64" - the standard macOS release-asset naming convention -
// which silently excluded every darwin asset from platform/arch matching.
// A side is only checked when the keyword doesn't already end/start with its
// own delimiter (e.g. "win-" already asserts its right edge via the hyphen).
func hasBoundedKeyword(name, kw string) bool {
	return len(boundedKeywordSpans(name, kw)) > 0
}

func boundedKeywordSpans(name, kw string) []keywordSpan {
	if kw == "" {
		return nil
	}
	kwStartsWithLetter := isLower(kw[0])
	kwEndsWithLetter := isLower(kw[len(kw)-1])
	var spans []keywordSpan
	start := 0
	for {
		rel := strings.Index(name[start:], kw)
		if rel == -1 {
			return spans
		}
		idx := start + rel
		beforeOK := !kwStartsWithLetter || idx == 0 || !isLower(name[idx-1])
		afterIdx := idx + len(kw)
		afterOK := !kwEndsWithLetter || afterIdx == len(name) || !isLower(name[afterIdx])
		if beforeOK && afterOK {
			spans = append(spans, keywordSpan{idx, afterIdx})
		}
		start = idx + 1
	}
}

func isLower(b byte) bool {
	return b >= 'a' && b <= 'z'
}

func isAmbiguousArchive(name string) bool {
	for _, ext := range catalog.AmbiguousArchives {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func sourceArchive(canonical string, platforms []Platform, arches []Arch) bool {
	for _, tok := range catalog.Source {
		if hasBoundedKeyword(canonical, tok) {
			return true
		}
	}
	return false
}
