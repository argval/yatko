package picker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/argval/yatko/github"
)

type corpusFile struct {
	Releases []CorpusRelease `json:"releases"`
}

// CorpusRelease is a snapshot of a real GitHub release plus labeled picks.
type CorpusRelease struct {
	Repo         string                             `json:"repo"`
	Tag          string                             `json:"tag"`
	Assets       []string                           `json:"assets"`
	Expect       []CorpusExpectation                `json:"expect"`
	TargetLabels map[string][]github.ArtifactTarget `json:"targetLabels"`
	Observed     map[string]github.ArtifactMetadata `json:"-"`
}

// CorpusExpectation is a human-labeled pick for one visitor platform/arch.
// Asset == nil means the picker must abstain. KnownMiss tracks a gap without
// failing the suite until a reviewed catalog change lands.
type CorpusExpectation struct {
	Platform  string  `json:"platform"`
	Arch      string  `json:"arch"`
	Prefer    string  `json:"prefer,omitempty"`
	Libc      string  `json:"libc,omitempty"`
	UserAgent string  `json:"userAgent,omitempty"`
	Asset     *string `json:"asset"`
	KnownMiss bool    `json:"knownMiss,omitempty"`
	Reason    string  `json:"reason,omitempty"`
}

// CorpusScore is the measurement surface for the labeled corpus.
type CorpusScore struct {
	Total               int
	Top1                int
	WrongPlatform       int
	WrongArch           int
	UnlabeledSelections int
	FalseAbstain        int
	FalseAuto           int
	Abstain             int
	KnownMiss           int
	KnownMissResolved   int
	DecideNanos         int64
}

// Decider picks an asset for one corpus case. The rule picker ignores repo;
// model-backed deciders (backend/jev) use it as context.
type Decider func(repo string, assets []github.Asset, platform Platform, arch Arch, opts PickOpts) AssetDecision

func ruleDecider(_ string, assets []github.Asset, platform Platform, arch Arch, opts PickOpts) AssetDecision {
	return DecideAsset(assets, platform, arch, opts)
}

// LoadCorpusFile reads the labeled release corpus (shared/picker/corpus.json).
func LoadCorpusFile(path string) ([]CorpusRelease, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file corpusFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if len(file.Releases) == 0 {
		return nil, fmt.Errorf("corpus has no releases")
	}
	var observed map[string]map[string]github.ArtifactMetadata
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(path), "corpus_observations.json")); err == nil {
		if err := json.Unmarshal(data, &observed); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	for i := range file.Releases {
		r := &file.Releases[i]
		r.Observed = observed[r.Repo+"@"+r.Tag]
	}
	return file.Releases, nil
}

func assetsFromNames(names []string) []github.Asset {
	out := make([]github.Asset, len(names))
	for i, name := range names {
		out[i] = github.Asset{Name: name}
	}
	return out
}

func scoreExpectation(rel CorpusRelease, exp CorpusExpectation, decide Decider) (gotName string, auto, wrongPlatform, wrongArch, unlabeled bool, elapsed time.Duration) {
	assets := assetsFromNames(rel.Assets)
	for i := range assets {
		if m, ok := rel.Observed[assets[i].Name]; ok {
			assets[i].Artifact = &m
		}
	}
	platform := Platform(exp.Platform)
	arch := Arch(exp.Arch)
	start := time.Now()
	d := decide(rel.Repo, assets, platform, arch, PickOpts{
		Prefer:    exp.Prefer,
		Libc:      ResolveLibc(exp.Libc),
		UserAgent: exp.UserAgent,
	})
	elapsed = time.Since(start)
	auto = d.ShouldAutoSelect()
	if d.Asset != nil {
		gotName = d.Asset.Name
		labels := rel.TargetLabels[gotName]
		unlabeled = len(labels) == 0
		platformMatch, archMatch := false, false
		for _, target := range labels {
			if target.Platform == exp.Platform {
				platformMatch = true
				// Wrong CPU means "cannot run", so listed emulation fallbacks count as a match.
				_, emulated := EmulationRank(platform, arch, Arch(target.Arch))
				if exp.Arch == "" || target.Arch == "" || target.Arch == exp.Arch || emulated {
					archMatch = true
				}
			}
		}
		wrongPlatform = !unlabeled && !platformMatch
		wrongArch = !unlabeled && platformMatch && !archMatch
	}
	return gotName, auto, wrongPlatform, wrongArch, unlabeled, elapsed
}

func scoreCorpus(releases []CorpusRelease) (CorpusScore, []string) {
	return ScoreCorpusWith(releases, ruleDecider)
}

// ScoreCorpusWith scores any decider against the labeled corpus, so a model
// can be measured with exactly the same metrics as the rule picker.
func ScoreCorpusWith(releases []CorpusRelease, decide Decider) (CorpusScore, []string) {
	var score CorpusScore
	var mismatches []string
	for _, rel := range releases {
		for _, exp := range rel.Expect {
			score.Total++
			gotName, auto, wrongPlatform, wrongArch, unlabeled, elapsed := scoreExpectation(rel, exp, decide)
			score.DecideNanos += elapsed.Nanoseconds()
			if auto && wrongPlatform {
				score.WrongPlatform++
			}
			if auto && wrongArch {
				score.WrongArch++
			}
			if auto && unlabeled {
				score.UnlabeledSelections++
				mismatches = append(mismatches, fmt.Sprintf("%s: selected %s without independent target labels", rel.Repo, gotName))
			}
			if !auto {
				score.Abstain++
			}

			wantAuto := exp.Asset != nil
			switch {
			case !wantAuto && auto:
				score.FalseAuto++
				if !exp.KnownMiss {
					mismatches = append(mismatches, fmt.Sprintf("%s %s/%s: auto-selected %s, want abstain", rel.Repo, exp.Platform, exp.Arch, gotName))
				} else {
					score.KnownMiss++
				}
			case wantAuto && !auto:
				score.FalseAbstain++
				if !exp.KnownMiss {
					mismatches = append(mismatches, fmt.Sprintf("%s %s/%s: abstained, want %s", rel.Repo, exp.Platform, exp.Arch, *exp.Asset))
				} else {
					score.KnownMiss++
				}
			case wantAuto && auto && gotName != *exp.Asset:
				if !exp.KnownMiss {
					mismatches = append(mismatches, fmt.Sprintf("%s %s/%s: got %s, want %s", rel.Repo, exp.Platform, exp.Arch, gotName, *exp.Asset))
				} else {
					score.KnownMiss++
				}
			default:
				if wantAuto && auto {
					score.Top1++
				}
				if exp.KnownMiss {
					score.KnownMissResolved++
				}
			}
		}
	}
	return score, mismatches
}
