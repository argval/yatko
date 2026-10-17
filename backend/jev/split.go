package jev

import (
	"context"
	"fmt"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

// maxFiles bounds one request: two questions per file must fit Jev's
// 64k-token per-request context (https://docs.typesafe.ai/models.md).
const maxFiles = 100

// Fit levels for the per-file Score question, low to high.
const (
	fitNone     = 0
	fitEmulated = 1
	fitNative   = 2
)

var fitLevels = []string{
	"Built for a different operating system, or for a CPU the visitor's computer cannot run, for example an ARM build on an x86-64 PC or a 32-bit x86 build on an ARM device.",
	"Built for the visitor's operating system but for a different CPU that the computer still runs through emulation: an x86-64 build on an arm64 Mac or an arm64 Windows PC, or a 32-bit x86 build on 64-bit Windows.",
	"Built for the visitor's operating system and natively for their CPU. A macOS universal build counts. A file that names the operating system but no CPU counts. When the visitor's CPU is unknown, an x86-64 build counts.",
}

// Each question names its file directly. Pointing at `files[i]` made Jev
// judge the wrong file: indexing into a long list is a known weak spot.
func fitInstructions(file string) map[string]string {
	return map[string]string{
		"file":     file,
		"question": "Can the `visitor` run `file` on their operating system and CPU?",
	}
}

func mainInstructions(file string) map[string]string {
	return map[string]string{
		"file": file,
		"question": "Is `file` the standard build of the main program of `repository`? `files` lists every file in the release. " +
			"Answer no for a companion tool (a separate CLI, server, editor extension such as .vsix, or plugin), " +
			"for a debug, symbols or profile build, and for a special variant such as baseline, portable or no-web. " +
			"When `visitor.libc` is given, answer no for a build with a different libc; otherwise answer no for a musl build.",
	}
}

// fileJudgment is Jev's view of one file plus the code-side format rank.
type fileJudgment struct {
	asset github.Asset
	fit   int     // most probable fit level
	fitP  float64 // probability of that level
	main  float64 // probability the file is the standard main build
	rank  int     // picker.FormatRank: lower is better
	// libcMiss marks a build for a libc the visitor did not ask for.
	libcMiss bool
}

// DecideFit was the second experiment: Jev judges each file's fit for this
// visitor (a Score) and whether it is the standard main build (a Noul); code
// applies the format policy. Decide (attrs.go) replaced it; kept for the eval.
func (c *Client) DecideFit(ctx context.Context, repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, Usage, error) {
	if _, ok := osNames[platform]; !ok {
		return picker.AssetDecision{Status: "unknown", Confidence: picker.ConfidenceLow, Reasons: []string{"unknown platform"}}, Usage{}, nil
	}
	candidates := installable(assets)
	if len(candidates) == 0 {
		return picker.AssetDecision{Status: "incompatible", Confidence: picker.ConfidenceLow, Reasons: []string{"no installable assets"}}, Usage{}, nil
	}
	if len(candidates) > maxFiles {
		return picker.AssetDecision{}, Usage{}, fmt.Errorf("jev: %d files exceeds per-request limit %d", len(candidates), maxFiles)
	}

	names := make([]string, len(candidates))
	questions := make(map[string]question, 2*len(candidates))
	for i, a := range candidates {
		names[i] = a.Name
		questions[fmt.Sprintf("fit_%d", i)] = question{Type: "score", Instructions: fitInstructions(a.Name), Criteria: fitLevels}
		questions[fmt.Sprintf("main_%d", i)] = question{Type: "noul", Instructions: mainInstructions(a.Name)}
	}
	resp, err := c.post(ctx, request{
		Model:     Model,
		State:     map[string]any{"repository": repo, "visitor": visitorState(platform, arch, opts), "files": names},
		Questions: questions,
	})
	if err != nil {
		return picker.AssetDecision{}, Usage{}, err
	}

	judged := make([]fileJudgment, len(candidates))
	for i, a := range candidates {
		fit, okF := resp.Answers[fmt.Sprintf("fit_%d", i)]
		main, okM := resp.Answers[fmt.Sprintf("main_%d", i)]
		if !okF || !okM {
			return picker.AssetDecision{}, Usage{}, fmt.Errorf("jev: response missing answers for file %d", i)
		}
		level, p := topLevel(fit.Probabilities)
		judged[i] = fileJudgment{asset: a, fit: level, fitP: p, main: main.Noul, rank: picker.FormatRank(a.Name, platform, opts)}
	}
	usage := Usage{Model: resp.Model, InputTokens: resp.Usage.InputTokens, Consistent: true}
	return choose(judged), usage, nil
}

// topLevel returns the most probable Score level; ties go to the lower,
// safer level so an even split never upgrades a file to "native".
func topLevel(probs map[string]float64) (int, float64) {
	best, bestP := fitNone, -1.0
	for level := fitNone; level <= fitNative; level++ {
		if p := probs[fmt.Sprint(level)]; p > bestP {
			best, bestP = level, p
		}
	}
	return best, bestP
}

// choose narrows the files in policy order: best fit tier, then the main
// build, then the best format rank, then Jev's certainty, then release order.
func choose(judged []fileJudgment) picker.AssetDecision {
	tier := fitNone
	for _, j := range judged {
		tier = max(tier, j.fit)
	}
	if tier == fitNone {
		return picker.AssetDecision{Status: "incompatible", Confidence: picker.ConfidenceLow, Reasons: []string{"jev: no file fits this OS and CPU"}}
	}
	pool := keep(judged, func(j fileJudgment) bool { return j.fit == tier })
	// Only narrow to main builds when one exists: all-musl or all-CLI releases still pick.
	if mains := keep(pool, func(j fileJudgment) bool { return j.main >= 0.5 }); len(mains) > 0 {
		pool = mains
	}
	if fits := keep(pool, func(j fileJudgment) bool { return !j.libcMiss }); len(fits) > 0 {
		pool = fits
	}
	bestRank := pool[0].rank
	for _, j := range pool {
		bestRank = min(bestRank, j.rank)
	}
	pool = keep(pool, func(j fileJudgment) bool { return j.rank == bestRank })
	win, runnerUp := pool[0], 0.0
	for _, j := range pool[1:] {
		if score(j) > score(win) {
			win, runnerUp = j, score(win)
		} else {
			runnerUp = max(runnerUp, score(j))
		}
	}

	conf := picker.ConfidenceLow
	switch {
	case runnerUp >= tieRatio*score(win):
		// Two files Jev rates the same after every policy step: a real tie
		// (e.g. one release shipping several different apps). Abstain, don't guess.
	case win.fit == fitNative && win.fitP >= highProbability && win.main >= 0.5:
		conf = picker.ConfidenceHigh
	case win.fitP >= mediumProbability:
		// Emulated picks stay medium: they run, but a native build may be missing from our view.
		conf = picker.ConfidenceMedium
	}

	var alts []picker.RankedAsset
	for _, j := range judged {
		if j.asset.Name != win.asset.Name && j.fit > fitNone {
			alts = append(alts, picker.RankedAsset{Asset: j.asset, Reasons: []string{describe(j)}})
		}
	}

	winAsset := win.asset
	return picker.AssetDecision{
		Status:       "selected",
		Asset:        &winAsset,
		Confidence:   conf,
		Reasons:      []string{describe(win)},
		Alternatives: alts,
		Facts:        picker.FactsForAsset(winAsset),
	}
}

// tieRatio: a runner-up within 20% of the winner's score counts as a tie.
// ponytail: one hand-set constant; tune it on shadow logs, not the 18-release corpus.
const tieRatio = 0.8

func score(j fileJudgment) float64 { return j.fitP * j.main }

func keep(js []fileJudgment, ok func(fileJudgment) bool) []fileJudgment {
	var out []fileJudgment
	for _, j := range js {
		if ok(j) {
			out = append(out, j)
		}
	}
	return out
}

func describe(j fileJudgment) string {
	return fmt.Sprintf("jev %q fit=%s p=%.2f main=%.2f format_rank=%d", j.asset.Name, []string{"none", "emulated", "native"}[j.fit], j.fitP, j.main, j.rank)
}
