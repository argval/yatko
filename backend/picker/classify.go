package picker

import "sort"

// ArtifactKind is the installable shape of a release asset, inferred from
// filename tokens and exclusive extensions. Unknown means we did not invent one.
type ArtifactKind string

const (
	KindInstaller  ArtifactKind = "installer"
	KindPackage    ArtifactKind = "package"
	KindArchive    ArtifactKind = "archive"
	KindExecutable ArtifactKind = "executable"
	KindUnknown    ArtifactKind = ""
)

// ArtifactFacts is structured metadata extracted from a filename. Tokens the
// catalog does not know stay absent — Classify never guesses a platform or arch
// just because a caller needs one.
type ArtifactFacts struct {
	Original  string
	Canonical string
	Platforms []Platform
	Arches    []Arch
	Libc      Libc
	Kind      ArtifactKind
	Extension string // format key without a leading dot, e.g. "tar.gz"
	Variants  []string
	Source    bool
	NonNative bool
	Evidence  []string
	// FormatPlatform is set only when the extension itself implies an OS
	// (exe → windows, dmg → macos). Generic archives leave it empty.
	FormatPlatform Platform
	// Independent libc markers so musl+static still scores as both.
	HasMusl   bool
	HasGNU    bool
	HasStatic bool
}

func (f ArtifactFacts) HasPlatform(p Platform) bool {
	for _, got := range f.Platforms {
		if got == p {
			return true
		}
	}
	return false
}

func (f ArtifactFacts) HasArch(a Arch) bool {
	if a == UnknownArch {
		return false
	}
	for _, got := range f.Arches {
		if got == a {
			return true
		}
	}
	return false
}

func (f ArtifactFacts) HasOtherPlatform(p Platform) bool {
	for _, got := range f.Platforms {
		if got != p {
			return true
		}
	}
	return false
}

func (f ArtifactFacts) HasOtherArch(want Arch) bool {
	if want == UnknownArch {
		return false
	}
	for _, got := range f.Arches {
		if got != want {
			return true
		}
	}
	return false
}

// incompatiblePlatform reports whether facts name a different OS that should
// exclude this asset from want. Filenames that carry both linux and android
// (NDK/cross builds) target android; a linux visitor still must not receive them.
func incompatiblePlatform(facts ArtifactFacts, want Platform) bool {
	if !facts.HasOtherPlatform(want) {
		return false
	}
	if want == Android && androidWithLinuxHost(facts) {
		return false
	}
	return true
}

func androidWithLinuxHost(f ArtifactFacts) bool {
	if !f.HasPlatform(Android) || !f.HasPlatform(Linux) {
		return false
	}
	for _, p := range f.Platforms {
		if p != Android && p != Linux {
			return false
		}
	}
	return true
}

type aliasHit struct {
	start, end int
	key        string
	kw         string
}

func selectLongestHits(hits []aliasHit) []aliasHit {
	sort.SliceStable(hits, func(i, j int) bool {
		li := hits[i].end - hits[i].start
		lj := hits[j].end - hits[j].start
		if li != lj {
			return li > lj
		}
		if hits[i].start != hits[j].start {
			return hits[i].start < hits[j].start
		}
		return hits[i].kw < hits[j].kw
	})
	var kept []aliasHit
	for _, h := range hits {
		overlap := false
		for _, k := range kept {
			if h.start < k.end && k.start < h.end {
				overlap = true
				break
			}
		}
		if !overlap {
			kept = append(kept, h)
		}
	}
	return kept
}

func archAliasHits(name string) []aliasHit {
	var hits []aliasHit
	for a, kws := range catalog.Architectures {
		for _, kw := range kws {
			for _, span := range boundedKeywordSpans(name, kw) {
				hits = append(hits, aliasHit{span.start, span.end, a, kw})
			}
		}
	}
	return selectLongestHits(hits)
}

// Classify extracts structured facts from a release asset filename.
func Classify(name string) ArtifactFacts {
	canonical := canonicalizeName(name)
	f := ArtifactFacts{
		Original:  name,
		Canonical: canonical,
		NonNative: isNonNative(canonical),
	}

	for p, entry := range catalog.Platforms {
		for _, kw := range entry.Aliases {
			if hasBoundedKeyword(canonical, kw) {
				f.Platforms = appendUniquePlatform(f.Platforms, Platform(p))
				f.Evidence = appendUnique(f.Evidence, kw)
				break
			}
		}
	}

	for _, hit := range archAliasHits(canonical) {
		f.Arches = appendUniqueArch(f.Arches, Arch(hit.key))
		f.Evidence = appendUnique(f.Evidence, hit.kw)
	}

	f.Source = sourceArchive(canonical, f.Platforms, f.Arches)

	if key, entry, ok := matchFormat(canonical); ok {
		f.Extension = key
		f.Kind = ArtifactKind(entry.Kind)
		if entry.Platform != "" {
			f.FormatPlatform = Platform(entry.Platform)
		}
		f.Evidence = appendUnique(f.Evidence, "."+key)
	}

	for libc, kws := range catalog.Libc {
		for _, kw := range kws {
			if hasBoundedKeyword(canonical, kw) {
				switch libc {
				case "musl":
					f.HasMusl = true
					f.Libc = LibcMusl
				case "gnu":
					f.HasGNU = true
					if f.Libc == LibcAny {
						f.Libc = LibcGNU
					}
				case "static":
					f.HasStatic = true
					if f.Libc == LibcAny {
						f.Libc = LibcStatic
					}
				}
				f.Evidence = appendUnique(f.Evidence, kw)
				break
			}
		}
	}

	for _, kw := range catalog.Variants {
		if hasBoundedKeyword(canonical, kw) {
			f.Variants = append(f.Variants, kw)
			f.Evidence = appendUnique(f.Evidence, kw)
		}
	}

	return f
}

func appendUnique(dst []string, v string) []string {
	for _, e := range dst {
		if e == v {
			return dst
		}
	}
	return append(dst, v)
}

func appendUniquePlatform(dst []Platform, p Platform) []Platform {
	for _, e := range dst {
		if e == p {
			return dst
		}
	}
	return append(dst, p)
}

func appendUniqueArch(dst []Arch, a Arch) []Arch {
	for _, e := range dst {
		if e == a {
			return dst
		}
	}
	return append(dst, a)
}

func formatIsGeneric(ext string) bool {
	entry, ok := catalog.Formats[ext]
	if !ok {
		return true
	}
	return entry.Platform == ""
}
