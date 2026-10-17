package picker

import (
	"fmt"

	"github.com/argval/yatko/github"
)

// Fit levels a model gives one file for a visitor, low to high.
const (
	FitNone     = 0 // wrong OS, or a CPU the computer cannot run
	FitEmulated = 1 // right OS, other CPU that runs through emulation
	FitNative   = 2 // right OS and native CPU (macOS universal counts)
)

// FileJudgment is a model's view of one file (Jev or the trained asset model)
// plus the code-side format rank. ChooseJudged turns a set of them into a decision.
type FileJudgment struct {
	Asset github.Asset
	Fit   int     // most probable fit level
	FitP  float64 // probability of that level
	Main  float64 // probability the file is the standard main build
	Rank  int     // FormatRank: lower is better
}

// Thresholds on the winner's fit probability, and the tie rule.
// ponytail: hand-set constants; tune them on shadow logs, not the 18-release corpus.
const (
	judgedHigh   = 0.8
	judgedMedium = 0.5
	tieRatio     = 0.8 // a runner-up within 20% of the winner's score is a tie
)

// Installable keeps every file a person could install. It drops only what is
// never a download target: source, debug, metadata, unknown formats (checksums.txt)
// and bare versioned archives with no OS/CPU token, which Yatko treats as source.
func Installable(assets []github.Asset) []github.Asset {
	var out []github.Asset
	for _, a := range assets {
		if a.State != "" && a.State != "uploaded" {
			continue
		}
		f := FactsForAsset(a)
		if f.Source || f.NonNative || f.Role == "debug" || f.Role == "metadata" {
			continue
		}
		if f.Extension == "" && !NativeBinaryName(f.Canonical) {
			continue
		}
		if f.Kind == KindArchive && len(f.Platforms) == 0 && len(f.Arches) == 0 {
			continue
		}
		out = append(out, a)
	}
	return out
}

// ChooseJudged narrows files in policy order: best fit tier, then the main
// build, then the best format rank, then the model's certainty. A tie after
// every step abstains. source ("jev", "ml") labels the reasons for logs.
func ChooseJudged(judged []FileJudgment, source string) AssetDecision {
	tier := FitNone
	for _, j := range judged {
		tier = max(tier, j.Fit)
	}
	if tier == FitNone {
		return AssetDecision{Status: "incompatible", Confidence: ConfidenceLow, Reasons: []string{source + ": no file fits this OS and CPU"}}
	}
	pool := keepJudged(judged, func(j FileJudgment) bool { return j.Fit == tier })
	// Only narrow to main builds when one exists: all-musl or all-CLI releases still pick.
	if mains := keepJudged(pool, func(j FileJudgment) bool { return j.Main >= 0.5 }); len(mains) > 0 {
		pool = mains
	}
	bestRank := pool[0].Rank
	for _, j := range pool {
		bestRank = min(bestRank, j.Rank)
	}
	pool = keepJudged(pool, func(j FileJudgment) bool { return j.Rank == bestRank })
	win, runnerUp := pool[0], 0.0
	for _, j := range pool[1:] {
		if judgedScore(j) > judgedScore(win) {
			win, runnerUp = j, judgedScore(win)
		} else {
			runnerUp = max(runnerUp, judgedScore(j))
		}
	}

	conf := ConfidenceLow
	switch {
	case runnerUp >= tieRatio*judgedScore(win):
		// Two files rated the same after every policy step: a real tie
		// (e.g. one release shipping several different apps). Abstain, don't guess.
	case win.Fit == FitNative && win.FitP >= judgedHigh && win.Main >= 0.5:
		conf = ConfidenceHigh
	case win.FitP >= judgedMedium:
		// Emulated picks stay medium: they run, but a native build may be missing from our view.
		conf = ConfidenceMedium
	}

	var alts []RankedAsset
	for _, j := range judged {
		if j.Asset.Name != win.Asset.Name && j.Fit > FitNone {
			alts = append(alts, RankedAsset{Asset: j.Asset, Reasons: []string{describeJudged(j, source)}})
		}
	}
	winAsset := win.Asset
	return AssetDecision{
		Status:       "selected",
		Asset:        &winAsset,
		Confidence:   conf,
		Reasons:      []string{describeJudged(win, source)},
		Alternatives: alts,
		Facts:        FactsForAsset(winAsset),
	}
}

func judgedScore(j FileJudgment) float64 { return j.FitP * j.Main }

func keepJudged(js []FileJudgment, ok func(FileJudgment) bool) []FileJudgment {
	var out []FileJudgment
	for _, j := range js {
		if ok(j) {
			out = append(out, j)
		}
	}
	return out
}

func describeJudged(j FileJudgment, source string) string {
	return fmt.Sprintf("%s %q fit=%s p=%.2f main=%.2f format_rank=%d", source, j.Asset.Name, []string{"none", "emulated", "native"}[j.Fit], j.FitP, j.Main, j.Rank)
}
