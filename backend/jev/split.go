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

// fitLevels are the Score levels, in picker.FitNone..FitNative order.
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

// Decide splits the decision: Jev judges each file on its own (a Score for
// OS/CPU fit and a Noul for "standard main build"), and code applies Yatko's
// format policy. Judging files one at a time removes Choice's option-order bias.
func (c *Client) Decide(ctx context.Context, repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) (picker.AssetDecision, Usage, error) {
	if _, ok := osNames[platform]; !ok {
		return picker.AssetDecision{Status: "unknown", Confidence: picker.ConfidenceLow, Reasons: []string{"unknown platform"}}, Usage{}, nil
	}
	candidates := picker.Installable(assets)
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

	judged := make([]picker.FileJudgment, len(candidates))
	for i, a := range candidates {
		fit, okF := resp.Answers[fmt.Sprintf("fit_%d", i)]
		main, okM := resp.Answers[fmt.Sprintf("main_%d", i)]
		if !okF || !okM {
			return picker.AssetDecision{}, Usage{}, fmt.Errorf("jev: response missing answers for file %d", i)
		}
		level, p := topLevel(fit.Probabilities)
		judged[i] = picker.FileJudgment{Asset: a, Fit: level, FitP: p, Main: main.Noul, Rank: picker.FormatRank(a.Name, platform, opts)}
	}
	usage := Usage{Model: resp.Model, InputTokens: resp.Usage.InputTokens, Consistent: true}
	return picker.ChooseJudged(judged, "jev"), usage, nil
}

// topLevel returns the most probable Score level; ties go to the lower,
// safer level so an even split never upgrades a file to "native".
func topLevel(probs map[string]float64) (int, float64) {
	best, bestP := picker.FitNone, -1.0
	for level := picker.FitNone; level <= picker.FitNative; level++ {
		if p := probs[fmt.Sprint(level)]; p > bestP {
			best, bestP = level, p
		}
	}
	return best, bestP
}
