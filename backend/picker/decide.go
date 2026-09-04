package picker

import "github.com/argval/yatko/github"

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
	family      int
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
			Confidence: ConfidenceLow,
			Reasons:    []string{"no release assets"},
		}
	}

	exts, ok := platformExts(platform)
	if !ok {
		return AssetDecision{
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
	for _, asset := range assets {
		facts := Classify(asset.Name)
		if facts.Source || facts.NonNative {
			continue
		}
		if incompatiblePlatform(facts, platform) {
			continue
		}

		rank, matched := extRankFor(facts.Canonical, exts, prefer)
		// Native release binaries commonly omit an extension (e.g. herdr-macos-aarch64).
		// Only accept them when the filename identifies this platform, so source files
		// and unrelated extensionless assets still stay out of the CTA.
		if !matched && facts.HasPlatform(platform) && (arch == UnknownArch || facts.HasArch(arch)) && !containsDot(facts.Canonical) {
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
			family:      archFamilyPenalty(facts, arch),
			bitWidth:    archBitWidth(facts),
			libc:        libcPenalty(facts, libc),
			variant:     len(facts.Variants),
		})
	}

	if len(candidates) == 0 {
		return AssetDecision{
			Confidence: ConfidenceLow,
			Reasons:    []string{"no matching installable asset"},
		}
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
				if !c.facts.HasOtherArch(arch) {
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

	best := candidates[0]
	for _, c := range candidates[1:] {
		if candidateBetter(
			c.extRank, c.family, c.bitWidth, c.libc, c.variant,
			best.extRank, best.family, best.bitWidth, best.libc, best.variant,
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
			c.extRank, c.family, c.bitWidth, c.libc, c.variant,
			best.extRank, best.family, best.bitWidth, best.libc, best.variant,
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
	confidence := confidenceFor(best, platform, arch, tied, len(candidates))

	return AssetDecision{
		Asset:        &asset,
		Confidence:   confidence,
		Reasons:      reasons,
		Alternatives: alternatives,
		Facts:        best.facts,
	}
}

func containsDot(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return true
		}
	}
	return false
}

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

func candidateEqual(ext, family, bits, libc, variant, bestExt, bestFamily, bestBits, bestLibc, bestVariant int) bool {
	return ext == bestExt && family == bestFamily && bits == bestBits && libc == bestLibc && variant == bestVariant
}

// archFamilyPenalty ranks how close facts' arch is to want when an exact
// match is unavailable. Lower is better. Same-family 64-bit (386→amd64,
// arm→arm64) beats the opposite family, so Windows 386 hosts don't get an
// arm64 build just because it was listed first.
func archFamilyPenalty(f ArtifactFacts, want Arch) int {
	if want == UnknownArch {
		return 0
	}
	if f.HasArch(want) {
		return 0
	}
	hasAMD64 := f.HasArch(AMD64)
	hasARM64 := f.HasArch(ARM64)
	hasX86 := f.HasArch(X86)
	hasARM := f.HasArch(ARM)
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
		if tagged {
			return 1
		}
		return 0
	}
}

func reasonsFor(c scored, platform Platform, arch Arch, tied bool) []string {
	var reasons []string
	if c.platformHit {
		reasons = append(reasons, "platform token matches "+string(platform))
	}
	if c.archHit {
		reasons = append(reasons, "arch token matches "+string(arch))
	}
	if c.facts.FormatPlatform == platform && c.facts.Extension != "" {
		reasons = append(reasons, "extension ."+c.facts.Extension+" implies "+string(platform))
	} else if c.facts.Extension != "" {
		reasons = append(reasons, "extension ."+c.facts.Extension)
	}
	if c.family > 0 && arch != UnknownArch {
		reasons = append(reasons, "cpu family fallback for "+string(arch))
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

func confidenceFor(best scored, platform Platform, arch Arch, tied bool, candidateCount int) Confidence {
	formatExclusive := best.facts.FormatPlatform == platform && best.facts.Extension != ""
	generic := formatIsGeneric(best.facts.Extension)
	platformEvidence := best.platformHit || formatExclusive
	archEvidence := arch == UnknownArch || best.archHit

	switch {
	case !platformEvidence && generic && !best.archHit:
		// foo.jar / untagged zip — no OS or arch evidence. Abstain.
		return ConfidenceLow
	case (best.platformHit || formatExclusive) && archEvidence && !tied:
		return ConfidenceHigh
	case formatExclusive && !tied:
		// .exe / .dmg / .apk without an arch token is still a strong OS signal.
		return ConfidenceHigh
	case tied && candidateCount > 1:
		return ConfidenceMedium
	case best.platformHit || formatExclusive || best.archHit:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}
