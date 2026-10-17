package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

// The fake server checks the wire format (option order survives, reverse is
// reversed, source files are filtered) and the decision mapping, with no paid call.
func TestDecideAveragesBothOrders(t *testing.T) {
	var got map[string]struct {
		Criteria json.RawMessage `json:"criteria"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Questions json.RawMessage `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body.Questions, &got); err != nil {
			t.Fatal(err)
		}
		// Forward leans on the first file, reverse on the second: average must pick arm64.
		w.Write([]byte(`{"model":"jev-1.13.0","answers":{
			"forward":{"type":"choice","choice":"app-x64.dmg","probabilities":{"app-x64.dmg":0.45,"app-arm64.dmg":0.4,"none of these files":0.15}},
			"reverse":{"type":"choice","choice":"app-arm64.dmg","probabilities":{"app-x64.dmg":0.05,"app-arm64.dmg":0.95,"none of these files":0}}
		},"usage":{"input_tokens":100}}`))
	}))
	defer srv.Close()
	c := &Client{apiKey: "test", endpoint: srv.URL, http: srv.Client()}

	assets := []github.Asset{{Name: "app-x64.dmg"}, {Name: "app-arm64.dmg"}, {Name: "app-1.0-src.tar.gz"}}
	d, usage, err := c.DecideChoice(context.Background(), "o/app", assets, picker.MacOS, picker.ARM64, picker.PickOpts{})
	if err != nil {
		t.Fatal(err)
	}

	wantFwd := `{"app-x64.dmg":null,"app-arm64.dmg":null,"none of these files":"No file fits this visitor's operating system and CPU."}`
	wantRev := `{"app-arm64.dmg":null,"app-x64.dmg":null,"none of these files":"No file fits this visitor's operating system and CPU."}`
	if string(got["forward"].Criteria) != wantFwd || string(got["reverse"].Criteria) != wantRev {
		t.Fatalf("criteria order:\n fwd %s\n rev %s", got["forward"].Criteria, got["reverse"].Criteria)
	}
	if d.Asset == nil || d.Asset.Name != "app-arm64.dmg" {
		t.Fatalf("asset = %+v, want app-arm64.dmg", d.Asset)
	}
	// p=0.675 and the two orders disagree: medium, never high.
	if usage.Consistent || d.Confidence != picker.ConfidenceMedium {
		t.Fatalf("consistent=%v confidence=%s, want false/medium", usage.Consistent, d.Confidence)
	}
}

func TestDecideNoneAbstains(t *testing.T) {
	d := decisionFrom(
		[]github.Asset{{Name: "app.exe"}},
		answer{Choice: NoneOption, Probabilities: map[string]float64{NoneOption: 0.9, "app.exe": 0.1}},
		answer{Choice: NoneOption, Probabilities: map[string]float64{NoneOption: 0.9, "app.exe": 0.1}},
		true,
	)
	if d.ShouldAutoSelect() || d.Status != "incompatible" {
		t.Fatalf("decision = %+v, want abstain", d)
	}
}

// TestChooseAppliesPolicyOrder checks the code half of the split: native beats
// emulated, main build beats a companion CLI, then format rank (.dmg before .zip).
func TestChooseAppliesPolicyOrder(t *testing.T) {
	j := func(name string, fit int, main float64) fileJudgment {
		return fileJudgment{asset: github.Asset{Name: name}, fit: fit, fitP: 0.9, main: main,
			rank: picker.FormatRank(name, picker.MacOS, picker.PickOpts{})}
	}
	d := choose([]fileJudgment{
		j("app-x64.dmg", fitEmulated, 0.9),
		j("app-cli-arm64.zip", fitNative, 0.1),
		j("app-arm64.zip", fitNative, 0.9),
		j("app-arm64.dmg", fitNative, 0.9),
		j("app-linux-arm64.tar.gz", fitNone, 0.9),
	})
	if d.Asset == nil || d.Asset.Name != "app-arm64.dmg" || d.Confidence != picker.ConfidenceHigh {
		t.Fatalf("got %v (%s), want app-arm64.dmg high", name(d), d.Confidence)
	}
	if d := choose([]fileJudgment{j("app.exe", fitNone, 0.9)}); d.ShouldAutoSelect() {
		t.Fatal("no fitting file must abstain")
	}
	// Several different apps that tie on every signal: abstain instead of guessing.
	if d := choose([]fileJudgment{j("ChatGPT.dmg", fitNative, 0.9), j("DeepSeek.dmg", fitNative, 0.85)}); d.ShouldAutoSelect() {
		t.Fatalf("tie must abstain, got %v (%s)", name(d), d.Confidence)
	}
}

// TestJevCorpus scores Jev against the labeled corpus next to the rule picker.
// It calls the paid API, so it runs only with JEV_EVAL=1 and an API key:
//
//	set -a; source ../.env; set +a; JEV_EVAL=1 go test ./jev -run TestJevCorpus -v -timeout 15m
func TestJevCorpus(t *testing.T) {
	c, _ := evalClient(t)
	releases, err := picker.LoadCorpusFile(sharedPath("corpus.json"))
	if err != nil {
		t.Fatal(err)
	}

	type decideFunc func(context.Context, string, []github.Asset, picker.Platform, picker.Arch, picker.PickOpts) (picker.AssetDecision, Usage, error)
	for _, variant := range []struct {
		label  string
		decide decideFunc
	}{{"attrs", c.Decide}, {"fit", c.DecideFit}, {"choice", c.DecideChoice}} {
		t.Run(variant.label, func(t *testing.T) { evalCorpus(t, releases, variant.decide) })
	}
}

func evalCorpus(t *testing.T, releases []picker.CorpusRelease, jevDecide func(context.Context, string, []github.Asset, picker.Platform, picker.Arch, picker.PickOpts) (picker.AssetDecision, Usage, error)) {
	var tokens, calls, inconsistent, failed int
	var disagree []string
	decide := func(repo string, assets []github.Asset, platform picker.Platform, arch picker.Arch, opts picker.PickOpts) picker.AssetDecision {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d, usage, err := jevDecide(ctx, repo, assets, platform, arch, opts)
		calls++
		tokens += usage.InputTokens
		if err != nil {
			failed++
			t.Logf("error %s %s/%s: %v", repo, platform, arch, err)
			return picker.AssetDecision{Confidence: picker.ConfidenceLow, Reasons: []string{err.Error()}}
		}
		if !usage.Consistent && d.Asset != nil {
			inconsistent++
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
		Model, score.Total, score.Top1, score.Abstain, score.FalseAbstain, score.FalseAuto, score.WrongPlatform, score.WrongArch, score.KnownMiss, score.KnownMissResolved)
	t.Logf("jev calls=%d errors=%d order_inconsistent=%d input_tokens=%d avg_latency=%s",
		calls, failed, inconsistent, tokens, time.Duration(score.DecideNanos/int64(max(score.Total, 1))))
	for _, m := range mismatches {
		t.Log("mismatch: " + m)
	}
	for _, d := range disagree {
		t.Log("vs rules: " + d)
	}
}

func name(d picker.AssetDecision) string {
	if d.Asset == nil {
		return "-"
	}
	return d.Asset.Name
}
