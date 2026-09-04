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

func TestDecideAsset_ArchOnlyZipIsMedium(t *testing.T) {
	assets := []github.Asset{
		{Name: "tool-x64.zip"},
		{Name: "tool-arm64.zip"},
	}
	d := DecideAsset(assets, Windows, AMD64, PickOpts{})
	if !d.ShouldAutoSelect() {
		t.Fatalf("arch-tagged zip should auto-select, confidence=%s reasons=%v", d.Confidence, d.Reasons)
	}
	if d.Asset.Name != "tool-x64.zip" {
		t.Fatalf("asset = %s", d.Asset.Name)
	}
	if d.Confidence != ConfidenceMedium {
		t.Fatalf("confidence = %s, want medium", d.Confidence)
	}
}

func TestDecideAsset_EmptyAssets(t *testing.T) {
	d := DecideAsset(nil, Linux, AMD64, PickOpts{})
	if d.ShouldAutoSelect() || d.Confidence != ConfidenceLow {
		t.Fatalf("got %+v", d)
	}
}
