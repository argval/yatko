package picker

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/argval/yatko/github"
)

type corpusFile struct {
	Releases []CorpusRelease `json:"releases"`
}

// CorpusRelease is a snapshot of a real GitHub release plus labeled picks.
type CorpusRelease struct {
	Repo   string              `json:"repo"`
	Tag    string              `json:"tag"`
	Assets []string            `json:"assets"`
	Expect []CorpusExpectation `json:"expect"`
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
}

// CorpusScore is the measurement surface for the labeled corpus.
type CorpusScore struct {
	Total             int
	Top1              int
	WrongPlatform     int
	FalseAbstain      int
	FalseAuto         int
	Abstain           int
	KnownMiss         int
	KnownMissResolved int
	DecideNanos       int64
}

func loadCorpusFile(path string) ([]CorpusRelease, error) {
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
	return file.Releases, nil
}

func assetsFromNames(names []string) []github.Asset {
	out := make([]github.Asset, len(names))
	for i, name := range names {
		out[i] = github.Asset{Name: name}
	}
	return out
}

func scoreExpectation(rel CorpusRelease, exp CorpusExpectation) (gotName string, auto bool, wrongPlatform bool, elapsed time.Duration) {
	assets := assetsFromNames(rel.Assets)
	platform := Platform(exp.Platform)
	arch := Arch(exp.Arch)
	start := time.Now()
	d := DecideAsset(assets, platform, arch, PickOpts{
		Prefer:    exp.Prefer,
		Libc:      ResolveLibc(exp.Libc),
		UserAgent: exp.UserAgent,
	})
	elapsed = time.Since(start)
	auto = d.ShouldAutoSelect()
	if d.Asset != nil {
		gotName = d.Asset.Name
		facts := Classify(gotName)
		if incompatiblePlatform(facts, platform) && facts.FormatPlatform != platform {
			wrongPlatform = true
		}
	}
	return gotName, auto, wrongPlatform, elapsed
}

func scoreCorpus(releases []CorpusRelease) (CorpusScore, []string) {
	var score CorpusScore
	var mismatches []string
	for _, rel := range releases {
		for _, exp := range rel.Expect {
			score.Total++
			gotName, auto, wrongPlatform, elapsed := scoreExpectation(rel, exp)
			score.DecideNanos += elapsed.Nanoseconds()
			if auto && wrongPlatform {
				score.WrongPlatform++
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
