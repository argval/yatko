package jev

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

// TestJevCorpus scores Decide against the labeled corpus next to the rule
// picker. It calls the paid API unless every request is already in the
// replay cache:
//
//	set -a; source ../.env; set +a; JEV_EVAL=1 go test ./jev -run TestJevCorpus -v -timeout 15m
//	JEV_EVAL=1 go test ./jev -run TestJevCorpus -v   # no key: replay only, misses are errors
//
// Decide fails the test on any wrong-platform, wrong-CPU or non-known-miss
// false auto-select, and on any service error; other misses are logged for tuning.
func TestJevCorpus(t *testing.T) {
	c, _ := evalClient(t)
	releases, err := picker.LoadCorpusFile(sharedPath("corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	model := c.ModelName()
	var tokens, calls, failed int
	var disagree []string
	decide := func(repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) picker.AssetDecision {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Corpus cases are explicit visitor labels, like ?platform=&arch=.
		opts.PlatformSource, opts.ArchSource = picker.SourceQuery, picker.SourceQuery
		d, usage, err := c.Decide(ctx, repo, assets, platform, arch, opts)
		calls++
		tokens += usage.InputTokens
		if err != nil {
			failed++
			t.Logf("error %s %s/%s: %v", repo, platform, arch, err)
			return picker.AssetDecision{Confidence: picker.ConfidenceLow, Reasons: []string{err.Error()}}
		}
		rule := picker.DecideAsset(assets, platform, arch, opts)
		if name(rule) != name(d) || rule.ShouldAutoSelect() != d.ShouldAutoSelect() {
			disagree = append(disagree, fmt.Sprintf("%s %s/%s prefer=%s libc=%s: rules=%s(%s) jev=%s(%s) %s",
				repo, platform, arch, opts.Prefer, opts.Libc, name(rule), rule.Confidence, name(d), d.Confidence, strings.Join(d.Reasons, "; ")))
		}
		return d
	}

	score, mismatches := picker.ScoreCorpusWith(releases, decide)
	t.Logf("jev model=%s cases=%d top1=%d abstain=%d false_abstain=%d false_auto=%d wrong_platform=%d wrong_arch=%d known_miss=%d known_miss_resolved=%d",
		model, score.Total, score.Top1, score.Abstain, score.FalseAbstain, score.FalseAuto, score.WrongPlatform, score.WrongArch, score.KnownMiss, score.KnownMissResolved)
	t.Logf("jev calls=%d errors=%d input_tokens=%d avg_latency=%s",
		calls, failed, tokens, time.Duration(score.DecideNanos/int64(max(score.Total, 1))))
	falseAuto := 0
	for _, m := range mismatches {
		t.Log("mismatch: " + m)
		if strings.Contains(m, "want abstain") { // ScoreCorpusWith omits known misses from mismatches
			falseAuto++
		}
	}
	for _, d := range disagree {
		t.Log("vs rules: " + d)
	}
	if score.WrongPlatform > 0 || score.WrongArch > 0 || falseAuto > 0 || failed > 0 {
		t.Errorf("unsafe: wrong_platform=%d wrong_arch=%d false_auto=%d (excluding known misses) errors=%d",
			score.WrongPlatform, score.WrongArch, falseAuto, failed)
	}
}

func name(d picker.AssetDecision) string {
	if d.Asset == nil {
		return "-"
	}
	return d.Asset.Name
}
