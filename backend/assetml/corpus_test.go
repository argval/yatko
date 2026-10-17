package assetml

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/argval/yatko/picker"
)

// TestCorpus scores the trained model on the labeled release corpus with the
// same metrics as the rule picker. It logs only: the model is experimental, so
// a retrain must not fail CI. Wrong OS/CPU counts are the numbers to watch.
func TestCorpus(t *testing.T) {
	path := filepath.Join("..", "..", "shared", "picker", "corpus.json")
	if _, err := os.Stat(path); err != nil {
		t.Skip("corpus not found")
	}
	releases, err := picker.LoadCorpusFile(path)
	if err != nil {
		t.Fatal(err)
	}
	score, mismatches := picker.ScoreCorpusWith(releases, Decide)
	t.Logf("ml cases=%d top1=%d abstain=%d false_abstain=%d false_auto=%d wrong_platform=%d wrong_arch=%d",
		score.Total, score.Top1, score.Abstain, score.FalseAbstain, score.FalseAuto, score.WrongPlatform, score.WrongArch)
	for _, m := range mismatches {
		t.Log("mismatch: " + m)
	}
}
