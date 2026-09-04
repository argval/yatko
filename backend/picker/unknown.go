package picker

import "strings"

var catalogKeywordList []string

func init() {
	catalogKeywordList = catalogKeywords()
}

func catalogKeywords() []string {
	seen := map[string]struct{}{}
	var kws []string
	add := func(kw string) {
		if kw == "" {
			return
		}
		if _, ok := seen[kw]; ok {
			return
		}
		seen[kw] = struct{}{}
		kws = append(kws, kw)
	}
	for _, e := range catalog.Platforms {
		for _, kw := range e.Aliases {
			add(kw)
		}
	}
	for _, aliases := range catalog.Architectures {
		for _, kw := range aliases {
			add(kw)
		}
	}
	for _, aliases := range catalog.Libc {
		for _, kw := range aliases {
			add(kw)
		}
	}
	for _, kw := range catalog.Variants {
		add(kw)
	}
	for _, kw := range catalog.NonNative {
		add(kw)
	}
	for _, kw := range catalog.Source {
		add(kw)
	}
	for key := range catalog.Formats {
		add(key)
	}
	return kws
}

// UnknownTokens returns filename tokens the catalog does not explain.
// Product names, distro codenames, and novel triples show up here so they can
// be reviewed into catalog.json — they never change a live download decision.
func UnknownTokens(name string) []string {
	canonical := canonicalizeName(name)
	if canonical == "" {
		return nil
	}
	covered := make([]bool, len(canonical))
	for _, kw := range catalogKeywordList {
		markBoundedMatches(covered, canonical, kw)
	}
	for i := 0; i < len(canonical); i++ {
		if canonical[i] >= '0' && canonical[i] <= '9' {
			covered[i] = true
		}
	}

	var tokens []string
	seen := map[string]struct{}{}
	i := 0
	for i < len(canonical) {
		if covered[i] || !isLower(canonical[i]) {
			i++
			continue
		}
		j := i
		for j < len(canonical) && !covered[j] && isLower(canonical[j]) {
			j++
		}
		tok := canonical[i:j]
		i = j
		if len(tok) < 2 {
			continue
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		tokens = append(tokens, tok)
	}
	return tokens
}

func markBoundedMatches(covered []bool, name, kw string) {
	if kw == "" {
		return
	}
	kwStartsWithLetter := isLower(kw[0])
	kwEndsWithLetter := isLower(kw[len(kw)-1])
	start := 0
	for {
		rel := strings.Index(name[start:], kw)
		if rel == -1 {
			return
		}
		idx := start + rel
		beforeOK := !kwStartsWithLetter || idx == 0 || !isLower(name[idx-1])
		afterIdx := idx + len(kw)
		afterOK := !kwEndsWithLetter || afterIdx == len(name) || !isLower(name[afterIdx])
		if beforeOK && afterOK {
			end := afterIdx
			if end > len(covered) {
				end = len(covered)
			}
			for i := idx; i < end; i++ {
				covered[i] = true
			}
		}
		start = idx + 1
	}
}

// HarvestUnknownTokens counts unexplained tokens across asset names.
// Review the high-frequency ones before copying anything into catalog.json.
func HarvestUnknownTokens(names []string) map[string]int {
	counts := map[string]int{}
	for _, name := range names {
		for _, tok := range UnknownTokens(name) {
			counts[tok]++
		}
	}
	return counts
}
