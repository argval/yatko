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

func TestDecideAsset_UntaggedExeNeedsCPUEvidence(t *testing.T) {
	d := DecideAsset([]github.Asset{{Name: "Setup.exe"}}, Windows, AMD64, PickOpts{})
	if d.ShouldAutoSelect() || d.Confidence != ConfidenceLow {
		t.Fatalf("OS-only filename cannot establish CPU compatibility: %+v", d)
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

// Obsidian v1.14.4: an OS-unknown arm64 tarball must not displace the
// confirmed-OS universal DMG/APK for ARM visitors.
func TestDecideAssetOSEvidenceBeatsArchOnlyName(t *testing.T) {
	assets := []github.Asset{
		{Name: "Obsidian-1.14.4.dmg", Artifact: &github.ArtifactMetadata{Targets: []github.ArtifactTarget{{Platform: "macos", Arch: "arm64"}, {Platform: "macos", Arch: "amd64"}}, Status: "inspected"}},
		{Name: "Obsidian-1.14.4.apk", Artifact: &github.ArtifactMetadata{ // live inspection result
			Targets: []github.ArtifactTarget{{Platform: "android", Arch: "arm64"}}, Role: "installer", Status: "inspected"}},
		{Name: "obsidian-1.14.4-arm64.tar.gz"},
		{Name: "Obsidian-1.14.4-arm64.AppImage"},
	}
	for _, tc := range []struct {
		platform Platform
		want     string
	}{{MacOS, "Obsidian-1.14.4.dmg"}, {Android, "Obsidian-1.14.4.apk"}, {Linux, "Obsidian-1.14.4-arm64.AppImage"}} {
		d := DecideAsset(assets, tc.platform, ARM64, PickOpts{})
		if !d.ShouldAutoSelect() || d.Asset.Name != tc.want {
			t.Fatalf("%s/arm64 = %+v, want %s", tc.platform, d, tc.want)
		}
	}
}

// clash-verge-rev v2.5.8: the bundled-WebView2 installer is a variant, not a tie.
func TestDecideAssetFixedWebView2IsVariant(t *testing.T) {
	assets := []github.Asset{{Name: "Clash.Verge_2.5.8_x64_fixed_webview2-setup.exe"}, {Name: "Clash.Verge_2.5.8_x64-setup.exe"}}
	d := DecideAsset(assets, Windows, AMD64, PickOpts{})
	if !d.ShouldAutoSelect() || d.Asset.Name != "Clash.Verge_2.5.8_x64-setup.exe" {
		t.Fatalf("%+v", d)
	}
}

// localsend v1.18.2 spells 64-bit ARM as "arm-64", like "x86-64".
func TestDecideAssetArmDash64IsARM64(t *testing.T) {
	assets := []github.Asset{{Name: "LocalSend-1.18.2-linux-arm-64.deb"}, {Name: "LocalSend-1.18.2-linux-x86-64.deb"}}
	if d := DecideAsset(assets, Linux, ARM64, PickOpts{}); !d.ShouldAutoSelect() || d.Asset.Name != assets[0].Name {
		t.Fatalf("arm64: %+v", d)
	}
	if d := DecideAsset(assets, Linux, ARM, PickOpts{}); d.ShouldAutoSelect() {
		t.Fatalf("64-bit ARM offered to 32-bit ARM: %+v", d)
	}
}

// localsend v1.18.2: the universal app DMG beats the arch-tagged companion CLI,
// while a CLI-only release still resolves normally.
func TestDecideAssetPrefersAppOverCompanionCLI(t *testing.T) {
	assets := []github.Asset{{Name: "LocalSend-1.18.2.dmg", Artifact: &github.ArtifactMetadata{Targets: []github.ArtifactTarget{{Platform: "macos", Arch: "amd64"}, {Platform: "macos", Arch: "arm64"}}, Status: "inspected"}}, {Name: "LocalSend-CLI-1.18.2-macos-arm-64.tar.gz"}, {Name: "LocalSend-CLI-1.18.2-macos-x86-64.tar.gz"}}
	for _, arch := range []Arch{ARM64, AMD64} {
		if d := DecideAsset(assets, MacOS, arch, PickOpts{}); !d.ShouldAutoSelect() || d.Asset.Name != "LocalSend-1.18.2.dmg" {
			t.Fatalf("macos/%s: %+v", arch, d)
		}
	}
	cliOnly := []github.Asset{{Name: "tool-cli-macos-arm64.tar.gz"}, {Name: "tool-cli-macos-x64.tar.gz"}}
	if d := DecideAsset(cliOnly, MacOS, ARM64, PickOpts{}); !d.ShouldAutoSelect() || d.Asset.Name != "tool-cli-macos-arm64.tar.gz" {
		t.Fatalf("CLI-only release: %+v", d)
	}
}

func TestDecideAssetPrefersInsomniaDesktopOverInso(t *testing.T) {
	assets := []github.Asset{
		{Name: "Insomnia.Core-13.1.0.exe", Artifact: &github.ArtifactMetadata{
			Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "386"}}, Role: "application", Status: "inspected",
		}},
		{Name: "inso-windows-x64-13.1.0.zip"},
	}
	d := DecideAsset(assets, Windows, AMD64, PickOpts{})
	if !d.ShouldAutoSelect() || d.Asset.Name != "Insomnia.Core-13.1.0.exe" {
		t.Fatalf("desktop pick: %+v", d)
	}
	if d := DecideAsset(assets[1:], Windows, AMD64, PickOpts{}); !d.ShouldAutoSelect() || d.Asset.Name != assets[1].Name {
		t.Fatalf("CLI-only release should remain available: %+v", d)
	}
}

func TestDecideAssetPrefersSetupOverPortable(t *testing.T) {
	assets := []github.Asset{
		{Name: "App-Setup.exe", Artifact: &github.ArtifactMetadata{
			Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "386"}}, Role: "application", Status: "inspected",
		}},
		{Name: "App-windows-x64-Portable.exe"},
	}
	d := DecideAsset(assets, Windows, AMD64, PickOpts{})
	if !d.ShouldAutoSelect() || d.Asset.Name != assets[0].Name {
		t.Fatalf("setup pick: %+v", d)
	}
	if d := DecideAsset(assets[1:], Windows, AMD64, PickOpts{}); !d.ShouldAutoSelect() {
		t.Fatalf("portable-only release should remain available: %+v", d)
	}
	unknownSetup := github.Asset{Name: "App-Setup.exe"}
	if d := DecideAsset([]github.Asset{unknownSetup, assets[1]}, Windows, AMD64, PickOpts{}); !d.ShouldAutoSelect() || d.Asset.Name != assets[1].Name {
		t.Fatalf("unknown setup CPU must not hide a compatible portable build: %+v", d)
	}
}

// CPU fallback policy: native (or universal on Mac) first, then the closest
// emulated build — Windows ARM64: x64 then x86; Windows x64: x86; Apple
// Silicon: Intel via Rosetta. Nothing runs "upward" (x86 never gets x64).
func TestDecideAssetCPUFallbackOrder(t *testing.T) {
	for _, tc := range []struct {
		platform Platform
		arch     Arch
		assets   []string
		want     string // "" = must not auto-select
	}{
		{Windows, ARM64, []string{"app-win-arm64.exe", "app-win-x64.exe", "app-win-x86.exe"}, "app-win-arm64.exe"},
		{Windows, ARM64, []string{"app-win-x86.exe", "app-win-x64.exe"}, "app-win-x64.exe"},
		{Windows, ARM64, []string{"app-win-x86.exe"}, "app-win-x86.exe"},
		{Windows, AMD64, []string{"app-win-x86.exe", "app-win-x64.exe"}, "app-win-x64.exe"},
		{Windows, AMD64, []string{"app-win-x86.exe"}, "app-win-x86.exe"},
		{Windows, AMD64, []string{"app-win-arm64.exe"}, ""},
		{Windows, X86, []string{"app-win-x64.exe"}, ""},
		{MacOS, ARM64, []string{"app-mac-x64.dmg", "app-mac-arm64.dmg", "app-mac-universal.dmg"}, "app-mac-universal.dmg"},
		{MacOS, ARM64, []string{"app-mac-x64.dmg", "app-mac-arm64.dmg"}, "app-mac-arm64.dmg"},
		{MacOS, ARM64, []string{"app-mac-x64.dmg"}, "app-mac-x64.dmg"},
		{MacOS, ARM64, []string{"app-mac-x64.dmg", "App.dmg"}, "App.dmg"}, // Untagged DMGs default to universal.
		{MacOS, AMD64, []string{"app-mac-x64.dmg", "app-mac-universal.dmg"}, "app-mac-universal.dmg"},
		{MacOS, AMD64, []string{"app-mac-arm64.dmg"}, ""},
		{MacOS, UnknownArch, []string{"app-mac-x64.dmg", "app-mac-arm64.dmg", "app-mac-universal.dmg"}, "app-mac-universal.dmg"},
		{Linux, ARM64, []string{"app-linux-x64.tar.gz"}, ""},
	} {
		var assets []github.Asset
		for _, name := range tc.assets {
			assets = append(assets, github.Asset{Name: name})
		}
		d := DecideAsset(assets, tc.platform, tc.arch, PickOpts{})
		got := ""
		if d.ShouldAutoSelect() {
			got = d.Asset.Name
		}
		if got != tc.want {
			t.Errorf("%s/%s %v: got %q, want %q (%v)", tc.platform, tc.arch, tc.assets, got, tc.want, d.Reasons)
		}
	}
	// Obsidian v1.14.4's NSIS stub inspects as x86 and must still reach x64/ARM64 Windows.
	stub := github.Asset{Name: "Obsidian-1.14.4.exe", Artifact: &github.ArtifactMetadata{
		Targets: []github.ArtifactTarget{{Platform: "windows", Arch: "386"}}, Role: "application", Status: "inspected"}}
	for _, arch := range []Arch{AMD64, ARM64} {
		if d := DecideAsset([]github.Asset{stub}, Windows, arch, PickOpts{}); !d.ShouldAutoSelect() {
			t.Errorf("windows/%s lost the x86 installer: %+v", arch, d)
		}
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
		{"EjectRemapper.zip", "macos", MacOS, false},
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
