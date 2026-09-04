package picker

import (
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func sharedPickerPath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "shared", "picker", name)
}

func TestCorpusMetrics(t *testing.T) {
	releases, err := loadCorpusFile(sharedPickerPath(t, "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	score, mismatches := scoreCorpus(releases)
	t.Logf(
		"corpus releases=%d cases=%d top1=%d abstain=%d false_abstain=%d false_auto=%d wrong_platform=%d known_miss=%d known_miss_resolved=%d avg_decide=%dns",
		len(releases),
		score.Total,
		score.Top1,
		score.Abstain,
		score.FalseAbstain,
		score.FalseAuto,
		score.WrongPlatform,
		score.KnownMiss,
		score.KnownMissResolved,
		score.DecideNanos/int64(max(score.Total, 1)),
	)
	if score.WrongPlatform != 0 {
		t.Errorf("wrong-platform picks: %d (must stay 0)", score.WrongPlatform)
	}
	if len(mismatches) > 0 {
		for _, m := range mismatches {
			t.Error(m)
		}
	}
}

func TestCorpusUnknownTokenHarvest(t *testing.T) {
	releases, err := loadCorpusFile(sharedPickerPath(t, "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, rel := range releases {
		names = append(names, rel.Assets...)
	}
	counts := HarvestUnknownTokens(names)
	type pair struct {
		tok   string
		count int
	}
	ranked := make([]pair, 0, len(counts))
	for tok, n := range counts {
		ranked = append(ranked, pair{tok, n})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].tok < ranked[j].tok
	})
	limit := 25
	if len(ranked) < limit {
		limit = len(ranked)
	}
	for _, p := range ranked[:limit] {
		t.Logf("alias_candidate token=%s count=%d", p.tok, p.count)
	}
	if len(counts) == 0 {
		t.Fatal("expected unexplained tokens in a real-release corpus")
	}
}

func TestUnknownTokens_SkipsCatalogEvidence(t *testing.T) {
	got := UnknownTokens("bun-darwin-aarch64.zip")
	for _, tok := range got {
		if tok == "darwin" || tok == "aarch64" || tok == "zip" {
			t.Fatalf("catalog token leaked into unknown set: %q in %v", tok, got)
		}
	}
}

func TestUnknownTokens_FindsUnexplainedProductName(t *testing.T) {
	got := UnknownTokens("bun-darwin-aarch64.zip")
	found := false
	for _, tok := range got {
		if tok == "bun" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want bun in %v", got)
	}
}
