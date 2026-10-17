package picker

import (
	"testing"

	"github.com/argval/yatko/github"
)

// TestChooseJudgedPolicyOrder: native beats emulated, the main build beats a
// companion CLI, then format rank (.dmg before .zip); ties abstain.
func TestChooseJudgedPolicyOrder(t *testing.T) {
	j := func(name string, fit int, main float64) FileJudgment {
		return FileJudgment{Asset: github.Asset{Name: name}, Fit: fit, FitP: 0.9, Main: main, Rank: FormatRank(name, MacOS, PickOpts{})}
	}
	d := ChooseJudged([]FileJudgment{
		j("app-x64.dmg", FitEmulated, 0.9),
		j("app-cli-arm64.zip", FitNative, 0.1),
		j("app-arm64.zip", FitNative, 0.9),
		j("app-arm64.dmg", FitNative, 0.9),
		j("app-linux-arm64.tar.gz", FitNone, 0.9),
	}, "test")
	if d.Asset == nil || d.Asset.Name != "app-arm64.dmg" || d.Confidence != ConfidenceHigh {
		t.Fatalf("got %+v, want app-arm64.dmg high", d)
	}
	if d := ChooseJudged([]FileJudgment{j("app.exe", FitNone, 0.9)}, "test"); d.ShouldAutoSelect() {
		t.Fatal("no fitting file must abstain")
	}
	// Several different apps that tie on every signal: abstain instead of guessing.
	if d := ChooseJudged([]FileJudgment{j("ChatGPT.dmg", FitNative, 0.9), j("DeepSeek.dmg", FitNative, 0.85)}, "test"); d.ShouldAutoSelect() {
		t.Fatalf("tie must abstain, got %+v", d)
	}
}
