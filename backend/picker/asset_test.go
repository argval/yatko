package picker

import (
	"testing"

	"github.com/argval/yatko/github"
)

// TestDecideAsset_DarwinNotMisdetectedAsWindows guards against a bug
// where the "win-" Windows keyword substring-matched inside "darwin-arm64"
// (dar-WIN-arm64), silently excluding every macOS asset that follows the
// common "<name>-darwin-<arch>" release-asset naming convention.
func TestDecideAsset_DarwinNotMisdetectedAsWindows(t *testing.T) {
	assets := []github.Asset{
		{Name: "Logseq-darwin-arm64-2.0.1.dmg"},
		{Name: "Logseq-darwin-x64-2.0.1.dmg"},
		{Name: "Logseq-win-arm64-2.0.1-nsis.exe"},
		{Name: "Logseq-linux-x86_64-2.0.1.AppImage"},
	}

	got := autoSelected(assets, MacOS, ARM64, PickOpts{})
	if got == nil {
		t.Fatal("expected a macOS asset, got nil")
	}
	if got.Name != "Logseq-darwin-arm64-2.0.1.dmg" {
		t.Fatalf("expected Logseq-darwin-arm64-2.0.1.dmg, got %s", got.Name)
	}
}

func TestDecideAsset_WindowsStillDetected(t *testing.T) {
	assets := []github.Asset{
		{Name: "Logseq-darwin-arm64-2.0.1.dmg"},
		{Name: "Logseq-win-arm64-2.0.1-nsis.exe"},
	}

	got := autoSelected(assets, Windows, ARM64, PickOpts{})
	if got == nil {
		t.Fatal("expected a Windows asset, got nil")
	}
	if got.Name != "Logseq-win-arm64-2.0.1-nsis.exe" {
		t.Fatalf("expected Logseq-win-arm64-2.0.1-nsis.exe, got %s", got.Name)
	}
}

func TestClassify_DarwinHyphenIsNotWindows(t *testing.T) {
	if Classify("logseq-darwin-arm64-2.0.1.dmg").HasOtherPlatform(MacOS) {
		t.Error("darwin filename should not mention another platform")
	}
	if !Classify("logseq-win-arm64-2.0.1.exe").HasOtherPlatform(MacOS) {
		t.Error("win- filename should still be recognised as another platform")
	}
}

// TestDecideAsset_PrefersVanillaOverProfile guards against preferring
// secondary builds (profile/debug/baseline) when a vanilla asset exists.
// Bun ships both bun-darwin-aarch64.zip and bun-darwin-aarch64-profile.zip;
// the profile build sorts first alphabetically ("-" < ".") and was wrongly
// picked as the default despite ~600× fewer downloads.
func TestDecideAsset_PrefersVanillaOverProfile(t *testing.T) {
	assets := []github.Asset{
		{Name: "bun-darwin-aarch64-profile.zip"},
		{Name: "bun-darwin-aarch64.zip"},
		{Name: "bun-darwin-x64-baseline-profile.zip"},
		{Name: "bun-darwin-x64-baseline.zip"},
		{Name: "bun-darwin-x64-profile.zip"},
		{Name: "bun-darwin-x64.zip"},
	}

	got := autoSelected(assets, MacOS, ARM64, PickOpts{})
	if got == nil {
		t.Fatal("expected a macOS arm64 asset, got nil")
	}
	if got.Name != "bun-darwin-aarch64.zip" {
		t.Fatalf("expected bun-darwin-aarch64.zip, got %s", got.Name)
	}

	got = autoSelected(assets, MacOS, AMD64, PickOpts{})
	if got == nil {
		t.Fatal("expected a macOS amd64 asset, got nil")
	}
	if got.Name != "bun-darwin-x64.zip" {
		t.Fatalf("expected bun-darwin-x64.zip, got %s", got.Name)
	}
}

func TestDecideAsset_FallsBackToVariantWhenOnlyOption(t *testing.T) {
	assets := []github.Asset{
		{Name: "tool-darwin-arm64-profile.zip"},
		{Name: "tool-linux-amd64.tar.gz"},
	}

	got := autoSelected(assets, MacOS, ARM64, PickOpts{})
	if got == nil {
		t.Fatal("expected the profile asset when it is the only macOS match")
	}
	if got.Name != "tool-darwin-arm64-profile.zip" {
		t.Fatalf("expected tool-darwin-arm64-profile.zip, got %s", got.Name)
	}
}

func TestCanonicalizeName_CamelCaseOSArch(t *testing.T) {
	cases := map[string]string{
		"gdx-liftoff-winX64.zip":   "gdx-liftoff-win-x64.zip",
		"gdx-liftoff-linuxX64.zip": "gdx-liftoff-linux-x64.zip",
		"tool-darwin-arm64.dmg":    "tool-darwin-arm64.dmg",
		"Foo.AppImage":             "foo.appimage",
		"macOS-arm64.dmg":          "macos-arm64.dmg",
	}
	for in, want := range cases {
		if got := canonicalizeName(in); got != want {
			t.Errorf("canonicalizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDetectPlatform_MobileBeforeDesktop(t *testing.T) {
	cases := []struct {
		ua   string
		want Platform
	}{
		{
			ua:   "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 Chrome/120.0.0.0 Mobile Safari/537.36",
			want: Android,
		},
		{
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1",
			want: IOS,
		},
		{
			ua:   "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1",
			want: IOS,
		},
		{
			ua:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15",
			want: MacOS,
		},
		{
			ua:   "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
			want: Linux,
		},
		{
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
			want: Windows,
		},
	}
	for _, tc := range cases {
		if got := DetectPlatform(tc.ua); got != tc.want {
			t.Errorf("DetectPlatform(%q) = %q, want %q", tc.ua, got, tc.want)
		}
	}
}

func TestResolvePlatform(t *testing.T) {
	macUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15"
	cases := []struct {
		param string
		ua    string
		want  Platform
	}{
		{param: "linux", ua: macUA, want: Linux},
		{param: "Windows", ua: macUA, want: Windows},
		{param: "darwin", ua: "", want: MacOS},
		{param: "mac", ua: "", want: MacOS},
		{param: "", ua: macUA, want: MacOS},
		{param: "garbage", ua: macUA, want: MacOS}, // unrecognised → UA
		{param: "", ua: "", want: Windows},         // unknown UA → Windows
		{param: "android", ua: macUA, want: Android},
		{param: "ios", ua: macUA, want: IOS},
	}
	for _, tc := range cases {
		if got := ResolvePlatform(tc.param, tc.ua); got != tc.want {
			t.Errorf("ResolvePlatform(%q, %q) = %q, want %q", tc.param, tc.ua, got, tc.want)
		}
	}
}

func TestResolvePrefer(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"deb", "deb"},
		{".DEB", "deb"},
		{"app-image", "appimage"},
		{"AppImage", "appimage"},
		{"tgz", "tar.gz"},
		{"garbage", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ResolvePrefer(tc.in); got != tc.want {
			t.Errorf("ResolvePrefer(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveLibc(t *testing.T) {
	cases := []struct {
		in   string
		want Libc
	}{
		{"musl", LibcMusl},
		{"gnu", LibcGNU},
		{"glibc", LibcGNU},
		{"static", LibcStatic},
		{"", LibcAny},
		{"nope", LibcAny},
	}
	for _, tc := range cases {
		if got := ResolveLibc(tc.in); got != tc.want {
			t.Errorf("ResolveLibc(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveLinuxPackagePrefer(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		{"Mozilla/5.0 (X11; Ubuntu; Linux x86_64) AppleWebKit/537.36", "deb"},
		{"Mozilla/5.0 (X11; Debian; Linux x86_64) AppleWebKit/537.36", "deb"},
		{"Mozilla/5.0 (X11; Fedora; Linux x86_64) AppleWebKit/537.36", "rpm"},
		{"Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0", ""},
		{"curl/8.5.0", ""},
	}
	for _, tc := range cases {
		if got := ResolveLinuxPackagePrefer(tc.ua); got != tc.want {
			t.Errorf("ResolveLinuxPackagePrefer(%q) = %q, want %q", tc.ua, got, tc.want)
		}
	}
}
