package picker

import (
	"testing"

	"github.com/argval/yatko/github"
)

func TestDecideAsset_HighConfidenceDarwin(t *testing.T) {
	assets := []github.Asset{
		{Name: "Logseq-darwin-arm64-2.0.1.dmg"},
		{Name: "Logseq-win-arm64-2.0.1-nsis.exe"},
	}
	d := DecideAsset(assets, MacOS, ARM64, PickOpts{})
	if !d.ShouldAutoSelect() {
		t.Fatalf("confidence = %s, reasons = %v", d.Confidence, d.Reasons)
	}
	if d.Confidence != ConfidenceHigh {
		t.Fatalf("confidence = %s, want high", d.Confidence)
	}
	if d.Asset == nil || d.Asset.Name != "Logseq-darwin-arm64-2.0.1.dmg" {
		t.Fatalf("asset = %v", d.Asset)
	}
}

func TestDecideAsset_GenericJarAbstains(t *testing.T) {
	assets := []github.Asset{{Name: "lib-1.0.0.jar"}}
	d := DecideAsset(assets, Windows, AMD64, PickOpts{})
	if d.ShouldAutoSelect() {
		t.Fatalf("should abstain, got %s %s", d.Confidence, d.Asset.Name)
	}
}

func TestDecideAsset_UntaggedExeIsHigh(t *testing.T) {
	assets := []github.Asset{{Name: "Setup.exe"}}
	d := DecideAsset(assets, Windows, AMD64, PickOpts{})
	if !d.ShouldAutoSelect() {
		t.Fatalf("exe should auto-select, confidence=%s reasons=%v", d.Confidence, d.Reasons)
	}
	if d.Asset.Name != "Setup.exe" {
		t.Fatalf("asset = %s", d.Asset.Name)
	}
}

func TestDecideAsset_ArchOnlyZipAbstains(t *testing.T) {
	assets := []github.Asset{
		{Name: "tool-x64.zip"},
		{Name: "tool-arm64.zip"},
	}
	d := DecideAsset(assets, Windows, AMD64, PickOpts{})
	if d.ShouldAutoSelect() {
		t.Fatalf("architecture cannot prove OS compatibility: %+v", d)
	}
}

func TestDecideAssetRejectsKnownIncompatibility(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform Platform
		arch     Arch
	}{
		{"tool-windows-arm64.exe", Windows, AMD64},
		{"tool-win64.msi", Windows, X86},
		{"tool-linux-arm64.tar.gz", Linux, AMD64},
		{"tool-freebsd-x64.zip", Linux, AMD64},
		{"tool-linux-x64.exe", Windows, AMD64},
		{"tool-windows-linux-x64.zip", Windows, AMD64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := DecideAsset([]github.Asset{{Name: tc.name}}, tc.platform, tc.arch, PickOpts{})
			if d.ShouldAutoSelect() {
				t.Fatalf("incompatible asset selected: %+v", d)
			}
		})
	}
}

func TestDecideAssetUnresolvedTieAbstainsInEitherOrder(t *testing.T) {
	assets := []github.Asset{{Name: "tool-windows-x64-client.exe"}, {Name: "tool-windows-x64-server.exe"}}
	for range 2 {
		d := DecideAsset(assets, Windows, AMD64, PickOpts{})
		if d.ShouldAutoSelect() {
			t.Fatalf("unresolved role tie selected: %+v", d)
		}
		assets[0], assets[1] = assets[1], assets[0]
	}
}

func TestDecideAssetOrdinaryFilenameForms(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform Platform
		arch     Arch
	}{
		{"ResourceTool-macos-arm64.dmg", MacOS, ARM64},
		{"tool-v1.2.3-linux-amd64", Linux, AMD64},
		{"tool-linux-amd64.tgz", Linux, AMD64},
		{"tool-linux-arm.zip", Linux, ARM},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := DecideAsset([]github.Asset{{Name: tc.name}}, tc.platform, tc.arch, PickOpts{})
			if !d.ShouldAutoSelect() {
				t.Fatalf("valid filename not selected: %+v", d)
			}
		})
	}
}

func TestDecideAsset_EmptyAssets(t *testing.T) {
	d := DecideAsset(nil, Linux, AMD64, PickOpts{})
	if d.ShouldAutoSelect() || d.Confidence != ConfidenceLow {
		t.Fatalf("got %+v", d)
	}
}

func TestDecideAsset_VerifiedMacZip(t *testing.T) {
	for _, tt := range []struct {
		name, verified string
		platform       Platform
		want           bool
	}{
		{"EjectRemapper.zip", "macos", MacOS, true},
		{"EjectRemapper.zip", "", MacOS, false},
		{"EjectRemapper.zip", "macos", Windows, false},
		{"EjectRemapper.zip", "macos", Linux, false},
		{"EjectRemapper-source.zip", "macos", MacOS, false},
		{"app-windows.zip", "macos", MacOS, false},
	} {
		t.Run(tt.name+"/"+tt.verified+"/"+string(tt.platform), func(t *testing.T) {
			d := DecideAsset([]github.Asset{{Name: tt.name, ArchivePlatform: tt.verified}}, tt.platform, ARM64, PickOpts{})
			if d.ShouldAutoSelect() != tt.want {
				t.Fatalf("decision = %+v, want auto-select %v", d, tt.want)
			}
		})
	}
}
