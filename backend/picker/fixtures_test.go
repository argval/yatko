package picker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/argval/yatko/github"
)

type fixtureFile struct {
	Cases []fixtureCase `json:"cases"`
}

type fixtureCase struct {
	Name     string   `json:"name"`
	Platform string   `json:"platform"`
	Arch     string   `json:"arch"`
	Assets   []string `json:"assets"`
	Expected *string  `json:"expected"`
}

func loadSharedFixtures(t *testing.T) []fixtureCase {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "shared", "picker", "fixtures.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixtures %s: %v", path, err)
	}
	var file fixtureFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	if len(file.Cases) == 0 {
		t.Fatal("fixtures.json has no cases")
	}
	return file.Cases
}

func platformFromFixture(t *testing.T, s string) Platform {
	t.Helper()
	switch s {
	case "windows":
		return Windows
	case "macos":
		return MacOS
	case "linux":
		return Linux
	default:
		t.Fatalf("unknown platform %q", s)
		return Unknown
	}
}

func archFromFixture(t *testing.T, s string) Arch {
	t.Helper()
	switch s {
	case "amd64":
		return AMD64
	case "arm64":
		return ARM64
	case "arm":
		return ARM
	case "386":
		return X86
	case "":
		return UnknownArch
	default:
		t.Fatalf("unknown arch %q", s)
		return UnknownArch
	}
}

func TestPickAssetForArch_SharedFixtures(t *testing.T) {
	for _, tc := range loadSharedFixtures(t) {
		t.Run(tc.Name, func(t *testing.T) {
			assets := make([]github.Asset, len(tc.Assets))
			for i, name := range tc.Assets {
				assets[i] = github.Asset{Name: name}
			}
			got := PickAssetForArch(assets, platformFromFixture(t, tc.Platform), archFromFixture(t, tc.Arch))
			if tc.Expected == nil {
				if got != nil {
					t.Fatalf("expected nil, got %s", got.Name)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %s, got nil", *tc.Expected)
			}
			if got.Name != *tc.Expected {
				t.Fatalf("expected %s, got %s", *tc.Expected, got.Name)
			}
		})
	}
}
