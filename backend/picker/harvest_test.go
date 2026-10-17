package picker

import (
	"os"
	"strings"
	"testing"
)

// TestHarvestRules is the free baseline: the rule picker on filename-only
// harvest cases, scored against inspected targets. Holdout prints totals only.
func TestHarvestRules(t *testing.T) {
	releases, err := LoadHarvest(sharedPickerPath(t, "harvest.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, split := range []string{"tune", "holdout"} {
		var part []HarvestRelease
		for _, r := range releases {
			if HarvestSplit(r.Repo) == split {
				part = append(part, r)
			}
		}
		s, lines := ScoreHarvestWith(part, ruleDecider, 1, false)
		t.Logf("rules split=%s releases=%d %s", split, len(part), s)
		warm, _ := ScoreHarvestWith(part, ruleDecider, 1, true)
		t.Logf("rules warm split=%s releases=%d %s", split, len(part), warm)
		if split == "tune" && testing.Verbose() {
			show := os.Getenv("HARVEST_SHOW") // extra substring to print, e.g. "abstained"
			for _, l := range lines {
				if strings.Contains(l, "WRONG") || (show != "" && strings.Contains(l, show)) {
					t.Log(l)
				}
			}
		}
	}
}
