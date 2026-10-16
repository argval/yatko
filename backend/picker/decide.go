package picker

import (
	"strings"

	"github.com/argval/yatko/github"
)

// Confidence is how sure the picker is that Asset is the right installable.
// Unknown tokens stay unknown: low confidence means abstain from auto-select.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// RankedAsset is a runner-up the caller can surface instead of auto-selecting.
type RankedAsset struct {
	Asset   github.Asset
	Reasons []string
}

// AssetDecision is the picker interface: a ranked choice plus why, not only a URL.
// Asset may be set on a low-confidence result (the best guess); ShouldAutoSelect
// reports whether handlers should redirect to it.
type AssetDecision struct {
	Status       string
	Asset        *github.Asset
	Confidence   Confidence
	Reasons      []string
	Alternatives []RankedAsset
	Facts        ArtifactFacts
}

// ShouldAutoSelect reports whether the decision is safe to turn into a download
// redirect. Low confidence or a missing asset must not silently ship a binary.
func (d AssetDecision) ShouldAutoSelect() bool {
	return d.Asset != nil && d.Confidence != ConfidenceLow
}

type scored struct {
	asset       github.Asset
	facts       ArtifactFacts
	extRank     int
	archHit     bool
	platformHit bool
	bitWidth    int
	libc        int
	variant     int
}

// DecideAsset classifies each release asset, ranks with the deterministic
// catalog rules, and returns a decision with confidence. It does not invent
// download URLs — Asset always comes from the GitHub payload.
func DecideAsset(assets []github.Asset, platform Platform, arch Arch, opts PickOpts) AssetDecision {
	if len(assets) == 0 {
		return AssetDecision{
			Status:     "incompatible",
			Confidence: ConfidenceLow,
			Reasons:    []string{"no release assets"},
		}
	}

	exts, ok := platformExts(platform)
	if !ok {
		return AssetDecision{
			Status:     "unknown",
			Confidence: ConfidenceLow,
			Reasons:    []string{"unknown platform"},
		}
	}
	if platform == Linux && opts.Prefer == "" && opts.UserAgent != "" {
		exts = linuxExtensionsForUA(opts.UserAgent)
	}

	prefer := ResolvePrefer(opts.Prefer)
	libc := opts.Libc
	if s := string(libc); s != "" {
		libc = ResolveLibc(s)
	}

	var candidates []scored
	unresolved := false
	var conflicts []string
	for _, asset := range assets {
		if asset.State != "" && asset.State != "uploaded" {
			continue
		}
		facts := FactsForAsset(asset)
		if facts.Source || facts.NonNative || facts.Role == "debug" || facts.Role == "metadata" {
			continue
		}
		if len(facts.Conflicts) > 0 {
			conflicts = append(conflicts, facts.Conflicts...)
			continue
		}
		if facts.Inspection == "failed" {
			unresolved = true
			continue
		}
		if len(facts.Targets) == 0 {
			unresolved = true
		}
		for _, t := range facts.Targets {
			if t.Platform == "" {
				unresolved = true
			}
		}
		var compatible bool
		facts, compatible = factsForTarget(facts, platform, arch, libc)
		if !compatible {
			continue
		}

		rank, matched := extRankFor(facts.Canonical, exts, prefer)
		// Native release binaries commonly omit an extension (e.g. herdr-macos-aarch64).
		// Only accept them when the filename identifies this platform, so source files
		// and unrelated extensionless assets still stay out of the CTA.
		if !matched && facts.HasPlatform(platform) && (arch == UnknownArch || facts.HasArch(arch)) &&
			(nativeBinaryName(facts.Canonical) || (facts.Inspection == "inspected" && facts.Role == "application")) {
			rank, matched = len(exts), true
		}
		if !matched {
			continue
		}
		candidates = append(candidates, scored{
			asset:       asset,
			facts:       facts,
			extRank:     rank,
			archHit:     arch != UnknownArch && facts.HasArch(arch),
			platformHit: facts.HasPlatform(platform) || facts.FormatPlatform == platform,
			bitWidth:    archBitWidth(facts),
			libc:        libcPenalty(facts, libc),
			variant:     len(facts.Variants),
		})
	}

	if len(candidates) == 0 {
		status, reasons := "incompatible", []string{"no matching installable asset"}
		if unresolved {
			status, reasons = "unknown", []string{"not enough evidence to determine a compatible download"}
		}
		if len(conflicts) > 0 {
			status, reasons = "ambiguous", conflicts
		}
		return AssetDecision{
			Status:     status,
			Confidence: ConfidenceLow,
			Reasons:    reasons,
		}
	}

	// Prefer an explicit CPU match over assets that leave the CPU unknown.
	if arch != UnknownArch {
		var exact []scored
		for _, c := range candidates {
			if c.archHit {
				exact = append(exact, c)
			}
		}
		if len(exact) > 0 {
			candidates = exact
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

	best := candidates[0]
	for _, c := range candidates[1:] {
		if candidateBetter(
			c.extRank, c.bitWidth, c.libc, c.variant,
			best.extRank, best.bitWidth, best.libc, best.variant,
		) {
			best = c
		}
	}

	tied := false
	alternatives := make([]RankedAsset, 0, min(3, len(candidates)-1))
	for _, c := range candidates {
		if c.asset.Name == best.asset.Name {
			continue
		}
		if candidateEqual(
			c.extRank, c.bitWidth, c.libc, c.variant,
			best.extRank, best.bitWidth, best.libc, best.variant,
		) {
			tied = true
		}
		if len(alternatives) < 3 {
			alternatives = append(alternatives, RankedAsset{
				Asset:   c.asset,
				Reasons: reasonsFor(c, platform, arch, false),
			})
		}
	}

	asset := best.asset
	reasons := reasonsFor(best, platform, arch, tied)
	confidence := confidenceFor(best, platform, arch, tied)
	status := "selected"
	if confidence == ConfidenceLow {
		status = "unknown"
	}
	if tied {
		status = "ambiguous"
	}

	return AssetDecision{
		Status:       status,
		Asset:        &asset,
		Confidence:   confidence,
		Reasons:      reasons,
		Alternatives: alternatives,
		Facts:        best.facts,
	}
}

// factsForTarget filters supported target tuples before ranking preferences.
func factsForTarget(f ArtifactFacts, platform Platform, arch Arch, libc Libc) (ArtifactFacts, bool) {
	var targets []github.ArtifactTarget
	for _, t := range f.Targets {
		if t.Platform != "" && t.Platform != string(platform) {
			continue
		}
		if arch != UnknownArch && t.Arch != "" && t.Arch != string(arch) {
			continue
		}
		if libc != LibcAny && t.Libc != "" && t.Libc != string(libc) && t.Libc != "static" {
			continue
		}
		targets = append(targets, t)
	}
	if len(f.Targets) > 0 && len(targets) == 0 {
		return f, false
	}
	f.Targets = targets
	f.Platforms, f.Arches = nil, nil
	for _, t := range targets {
		if t.Platform != "" {
			f.Platforms = appendUniquePlatform(f.Platforms, Platform(t.Platform))
		}
		if t.Arch != "" {
			f.Arches = appendUniqueArch(f.Arches, Arch(t.Arch))
		}
	}
	return f, true
}

func nativeBinaryName(name string) bool {
	if !containsDot(name) {
		return true
	}
	// Version dots are not file extensions; checksum and signature suffixes are.
	suffix := name[strings.LastIndexByte(name, '.')+1:]
	return len(suffix) > 0 && suffix[0] >= '0' && suffix[0] <= '9'
}

func containsDot(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return true
		}
	}
	return false
}

func candidateBetter(ext, bits, libc, variant, bestExt, bestBits, bestLibc, bestVariant int) bool {
	if ext != bestExt {
		return ext < bestExt
	}
	if bits != bestBits {
		return bits < bestBits
	}
	if libc != bestLibc {
		return libc < bestLibc
	}
	return variant < bestVariant
}

func candidateEqual(ext, bits, libc, variant, bestExt, bestBits, bestLibc, bestVariant int) bool {
	return ext == bestExt && bits == bestBits && libc == bestLibc && variant == bestVariant
}

// archBitWidth ranks 64-bit above unspecified above 32-bit. Used when the
// client arch is unknown so we still prefer win64 over win32.
func archBitWidth(f ArtifactFacts) int {
	if f.HasArch(AMD64) || f.HasArch(ARM64) {
		return 0
	}
	if f.HasArch(X86) || f.HasArch(ARM) {
		return 2
	}
	return 1
}

// libcPenalty ranks classified libc/linkage markers. Lower is better.
// When want is LibcAny, prefer untagged names over musl/gnu tags.
// Explicit musl/gnu/static treats static as compatible with musl or gnu.
func libcPenalty(f ArtifactFacts, want Libc) int {
	tagged := f.HasMusl || f.HasGNU
	switch want {
	case LibcMusl:
		if f.HasMusl || f.HasStatic {
			return 0
		}
		if f.HasGNU {
			return 2
		}
		return 1
	case LibcGNU:
		if f.HasGNU || f.HasStatic {
			return 0
		}
		if f.HasMusl {
			return 2
		}
		return 1
	case LibcStatic:
		if f.HasStatic {
			return 0
		}
		return 1
	default:
		if f.HasMusl && !f.HasGNU {
			return 2
		}
		if tagged {
			return 1
		}
		return 0
	}
}

func reasonsFor(c scored, platform Platform, arch Arch, tied bool) []string {
	var reasons []string
	if c.platformHit {
		if c.asset.ArchivePlatform == string(platform) {
			reasons = append(reasons, "ZIP contains a "+string(platform)+" app bundle")
		} else {
			reasons = append(reasons, "platform token matches "+string(platform))
		}
	}
	for _, clue := range c.facts.Evidence {
		if strings.Contains(clue, ": ") {
			reasons = append(reasons, clue)
		}
	}
	if c.archHit {
		reasons = append(reasons, "arch token matches "+string(arch))
	}
	if c.facts.FormatPlatform == platform && c.facts.Extension != "" {
		reasons = append(reasons, "extension ."+c.facts.Extension+" implies "+string(platform))
	} else if c.facts.Extension != "" {
		reasons = append(reasons, "extension ."+c.facts.Extension)
	}
	if c.variant > 0 {
		reasons = append(reasons, "secondary build variant")
	}
	if tied {
		reasons = append(reasons, "tied with another candidate")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "extension matched "+string(platform)+" defaults")
	}
	return reasons
}

func confidenceFor(best scored, platform Platform, arch Arch, tied bool) Confidence {
	formatExclusive := best.facts.FormatPlatform == platform && best.facts.Extension != ""
	generic := formatIsGeneric(best.facts.Extension)
	platformEvidence := best.platformHit || formatExclusive
	archEvidence := arch == UnknownArch || best.archHit

	switch {
	case tied:
		return ConfidenceLow
	case !platformEvidence && generic:
		// foo.jar / untagged zip — no OS or arch evidence. Abstain.
		return ConfidenceLow
	case (best.platformHit || formatExclusive) && archEvidence && !tied:
		return ConfidenceHigh
	case formatExclusive && !tied:
		// .exe / .dmg / .apk without an arch token is still a strong OS signal.
		return ConfidenceHigh
	case best.platformHit || formatExclusive || best.archHit:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}
