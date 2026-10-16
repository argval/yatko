package github

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestReleaseMetadata(t *testing.T) {
	s := newFakeDownloads(t)
	var assets []Asset
	for _, name := range []string{"dist-manifest.json", "latest-mac.yml", "latest.yml", "latest-linux-arm64.yml"} {
		assets = append(assets, s.add(name, fixture(t, name)))
	}
	assets = append(assets, s.add("latest-linux.yml", []byte("version: [\n")))
	for _, name := range []string{
		"axolotlsay-aarch64-unknown-linux-musl.tar.gz", "axolotlsay-aarch64-unknown-linux-musl.tar.gz.sha256",
		"axolotlsay-x86_64-pc-windows-msvc.msi", "axolotlsay-installer.sh", "axolotlsay-universal2-apple-darwin.tar.gz",
		"axolotlsay-x86_64-unknown-linux-gnu-update", "axolotlsay-x86_64-unknown-linux-gnu.tar.gz.debug",
		"source.tar.gz", "future.bin", "App-1.0.0-arm64-mac.zip", "App-1.0.0-mac.zip", "App-1.0.0.dmg",
		"App Setup 1.0.0.exe", "app-1.0.0-x64.nsis.7z", "app-1.0.0-arm64.nsis.7z", "App-1.0.0-arm64.AppImage",
	} {
		assets = append(assets, s.add(name, []byte("asset bytes are never fetched")))
	}

	got, err := s.client().ReleaseMetadata(context.Background(), assets)
	if !errors.Is(err, ErrMalformedArtifact) {
		t.Fatalf("err = %v, want malformed latest-linux.yml", err)
	}
	if s.requests != 5 {
		t.Fatalf("requests = %d, want one per manifest", s.requests)
	}
	for name, want := range map[string]struct {
		role     string
		targets  []ArtifactTarget
		conflict bool
	}{
		"axolotlsay-aarch64-unknown-linux-musl.tar.gz":        {"application", []ArtifactTarget{{"linux", "arm64", "musl"}}, false},
		"axolotlsay-aarch64-unknown-linux-musl.tar.gz.sha256": {"metadata", []ArtifactTarget{{"linux", "arm64", "musl"}}, false},
		"axolotlsay-x86_64-pc-windows-msvc.msi":               {"installer", []ArtifactTarget{{"windows", "amd64", ""}}, false},
		"axolotlsay-installer.sh": {"installer", []ArtifactTarget{
			{"macos", "arm64", ""}, {"linux", "amd64", "gnu"}, {"linux", "amd64", "static"}, {"linux", "riscv64", "gnu"},
		}, false},
		"axolotlsay-universal2-apple-darwin.tar.gz":        {"application", []ArtifactTarget{{"macos", "amd64", ""}, {"macos", "arm64", ""}}, false},
		"axolotlsay-x86_64-unknown-linux-gnu-update":       {"metadata", []ArtifactTarget{{"linux", "amd64", "gnu"}}, false},
		"axolotlsay-x86_64-unknown-linux-gnu.tar.gz.debug": {"debug", []ArtifactTarget{{"linux", "amd64", "gnu"}}, false},
		"source.tar.gz":            {"source", nil, false},
		"future.bin":               {"metadata", []ArtifactTarget{{"target:wasip2", "wasm32", ""}}, false},
		"dist-manifest.json":       {"metadata", nil, false},
		"App-1.0.0-arm64-mac.zip":  {"unknown", []ArtifactTarget{{Platform: "macos"}}, false},
		"App Setup 1.0.0.exe":      {"unknown", []ArtifactTarget{{Platform: "windows"}}, false},
		"app-1.0.0-x64.nsis.7z":    {"unknown", []ArtifactTarget{{"windows", "amd64", ""}}, false},
		"app-1.0.0-arm64.nsis.7z":  {"unknown", []ArtifactTarget{{"windows", "arm64", ""}}, false},
		"App-1.0.0-arm64.AppImage": {"unknown", []ArtifactTarget{{"linux", "arm64", ""}}, false},
	} {
		m, ok := got[name]
		if !ok || m.Role != want.role || !slices.Equal(m.Targets, want.targets) || (len(m.Conflicts) > 0) != want.conflict || len(m.Evidence) == 0 {
			t.Errorf("%s: ok=%v %+v", name, ok, m)
		}
	}
	// Listed by two channel files with different platforms.
	if m := got["App-1.0.0-mac.zip"]; len(m.Conflicts) == 0 || len(m.Targets) != 2 {
		t.Errorf("mac zip: %+v", m)
	}
	// Not uploaded (Homebrew formula), or referenced only by path traversal / foreign URL.
	for _, name := range []string{"axolotlsay.rb", "App-1.0.0.dmg"} {
		if m, ok := got[name]; ok {
			t.Errorf("%s should be absent: %+v", name, m)
		}
	}
	if m := got["latest-linux.yml"]; m.Status != "failed" || m.Role != "metadata" {
		t.Errorf("malformed manifest: %+v", m)
	}
}

func TestReleaseMetadataRejectsUnusableManifests(t *testing.T) {
	s := newFakeDownloads(t)
	assets := []Asset{
		s.add("dist-manifest.json", []byte(`{"dist_version":"0.30.0","artifacts":[]}`)),
		s.add("latest-mac.yml", []byte("releaseDate: x\n")),
		{Name: "latest.yml", BrowserDownloadURL: "https://example.com/latest.yml", Size: 10},
		s.add("beta-linux.yml", make([]byte, maxUpdateYAMLBytes+1)),
	}
	got, err := s.client().ReleaseMetadata(context.Background(), assets)
	if !errors.Is(err, ErrMalformedArtifact) || s.requests != 2 {
		t.Fatalf("err=%v requests=%d", err, s.requests)
	}
	for _, a := range assets {
		if got[a.Name].Status != "failed" {
			t.Errorf("%s: %+v", a.Name, got[a.Name])
		}
	}
}

func TestTripleTargets(t *testing.T) {
	for triple, want := range map[string][]ArtifactTarget{
		"x86_64-pc-windows-gnu":         {{"windows", "amd64", ""}},
		"armv7-unknown-linux-gnueabihf": {{"linux", "arm", "gnu"}},
		"i686-unknown-linux-musl":       {{"linux", "386", "musl"}},
		"aarch64-linux-android":         {{"android", "arm64", ""}},
		"aarch64-apple-ios":             {{"ios", "arm64", ""}},
		"aarch64-apple-ios-sim":         {{"ios", "arm64", ""}},
		"x86_64-unknown-freebsd":        {{"freebsd", "amd64", ""}},
		"wasm32-unknown-unknown":        {{"target:unknown-unknown", "wasm32", ""}},
		"garbage":                       nil,
	} {
		if got := tripleTargets(triple); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", triple, got, want)
		}
	}
}

// Explicit unsupported targets must not become missing target fields.
func TestManifestUnsupportedTargetsRemainConstraints(t *testing.T) {
	for _, triple := range []string{"powerpc64le-unknown-linux-gnu", "x86_64-unknown-haiku", "aarch64-apple-ios-sim"} {
		got := tripleTargets(triple)
		if len(got) != 1 || got[0].Platform == "" || got[0].Arch == "" {
			t.Fatalf("%s: %+v", triple, got)
		}
	}
}

func TestElectronRejectsYAMLReferences(t *testing.T) {
	for _, data := range []string{"version: &v 1.0.0\npath: *v\n", "version: 1.0.0\nfiles: &files [{url: app.zip}]\n"} {
		if _, err := parseElectronUpdate("latest.yml", []byte(data), ArtifactTarget{Platform: "windows"}, inventory{}); !errors.Is(err, ErrMalformedArtifact) {
			t.Fatalf("expected bounded rejection, got %v", err)
		}
	}
}
