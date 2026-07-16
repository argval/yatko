package picker

import (
	"testing"

	"github.com/argval/yatko/github"
)

// TestPickAssetForArch_DarwinNotMisdetectedAsWindows guards against a bug
// where the "win-" Windows keyword substring-matched inside "darwin-arm64"
// (dar-WIN-arm64), silently excluding every macOS asset that follows the
// common "<name>-darwin-<arch>" release-asset naming convention.
func TestPickAssetForArch_DarwinNotMisdetectedAsWindows(t *testing.T) {
	assets := []github.Asset{
		{Name: "Logseq-darwin-arm64-2.0.1.dmg"},
		{Name: "Logseq-darwin-x64-2.0.1.dmg"},
		{Name: "Logseq-win-arm64-2.0.1-nsis.exe"},
		{Name: "Logseq-linux-x86_64-2.0.1.AppImage"},
	}

	got := PickAssetForArch(assets, MacOS, ARM64)
	if got == nil {
		t.Fatal("expected a macOS asset, got nil")
	}
	if got.Name != "Logseq-darwin-arm64-2.0.1.dmg" {
		t.Fatalf("expected Logseq-darwin-arm64-2.0.1.dmg, got %s", got.Name)
	}
}

func TestPickAssetForArch_WindowsStillDetected(t *testing.T) {
	assets := []github.Asset{
		{Name: "Logseq-darwin-arm64-2.0.1.dmg"},
		{Name: "Logseq-win-arm64-2.0.1-nsis.exe"},
	}

	got := PickAssetForArch(assets, Windows, ARM64)
	if got == nil {
		t.Fatal("expected a Windows asset, got nil")
	}
	if got.Name != "Logseq-win-arm64-2.0.1-nsis.exe" {
		t.Fatalf("expected Logseq-win-arm64-2.0.1-nsis.exe, got %s", got.Name)
	}
}

func TestMentionsOtherPlatform_DarwinHyphenBoundary(t *testing.T) {
	if mentionsOtherPlatform("logseq-darwin-arm64-2.0.1.dmg", MacOS) {
		t.Error("darwin filename should not mention another platform")
	}
	if !mentionsOtherPlatform("logseq-win-arm64-2.0.1.exe", MacOS) {
		t.Error("win- filename should still be recognised as another platform")
	}
}
