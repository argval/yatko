package github

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestTauriManifestOnlyDescribesCurrentReleaseAssets(t *testing.T) {
	s := newFakeDownloads(t)
	mac := s.add("opaque.zip", []byte("not downloaded"))
	win := s.add("setup.msi", []byte("not downloaded"))
	doc := map[string]any{"version": "1.2.3", "platforms": map[string]any{
		"darwin-aarch64": map[string]string{"url": mac.BrowserDownloadURL, "signature": "signature"},
		"darwin-x86_64":  map[string]string{"url": mac.BrowserDownloadURL, "signature": "signature"},
		"windows-x86_64": map[string]string{"url": win.BrowserDownloadURL, "signature": "signature"},
		"linux-x86_64":   map[string]string{"url": "https://evil.example/setup.msi", "signature": "signature"},
		"linux-aarch64":  map[string]string{"url": "../opaque.zip", "signature": "signature"},
	}}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	manifest := s.add("latest.json", b)
	got, err := s.client().ReleaseMetadata(context.Background(), []Asset{mac, win, manifest})
	if err != nil {
		t.Fatal(err)
	}
	if s.requests != 1 {
		t.Fatalf("downloaded payload assets: %d requests", s.requests)
	}
	if m := got[mac.Name]; m.Role != "application" || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "macos", Arch: "arm64"}, {Platform: "macos", Arch: "amd64"}}) {
		t.Fatalf("Mac: %+v", m)
	}
	if m := got[win.Name]; !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "windows", Arch: "amd64"}}) {
		t.Fatalf("Windows: %+v", m)
	}
	if got[manifest.Name].Role != "metadata" {
		t.Fatal("manifest is not a download")
	}
}

func TestTauriCustomTargetsAndMalformedSchemas(t *testing.T) {
	for _, key := range []string{"macos-universal", "darwin-aarch64-custom", "linux-mips64", "unknown"} {
		if target := tauriTarget(key); target.Platform != "tauri-target:"+key {
			t.Fatalf("invented meaning for %s: %+v", key, target)
		}
	}
	for _, data := range []string{`{}`, `{"version":"1","platforms":{}}`, `{"version":"1","platforms":{"windows-x86_64":{"url":"app.msi"}}}`, `{"version":true}`} {
		if _, err := parseTauriUpdate("latest.json", []byte(data), inventory{}); !errors.Is(err, ErrMalformedArtifact) {
			t.Fatalf("expected malformed schema, got %v", err)
		}
	}
}

func TestTauriManifestTargetConflictsRemainVisible(t *testing.T) {
	dist := ArtifactMetadata{Role: "application", Status: "inspected", Targets: []ArtifactTarget{{Platform: "windows", Arch: "amd64"}}}
	out := map[string]ArtifactMetadata{"app.zip": dist}
	mergeManifestFact(out, "app.zip", ArtifactMetadata{Role: "application", Status: "inspected", Targets: []ArtifactTarget{{Platform: "macos", Arch: "amd64"}}})
	if len(out["app.zip"].Conflicts) == 0 {
		t.Fatal("publisher disagreement disappeared")
	}
}

// clash-verge-rev v2.5.8: tauri-action writes REST asset URLs and OS-ARCH-BUNDLE keys.
func TestTauriManifestRESTAssetURLsAndBundleKeys(t *testing.T) {
	s := newFakeDownloads(t)
	mac := s.add("App_aarch64.app.tar.gz", []byte("not downloaded"))
	mac.ID = 623652157
	other := s.add("App_x64.app.tar.gz", []byte("not downloaded"))
	other.ID = 42
	api := "https://api.github.com/repos/acme/app/releases/assets/"
	doc := map[string]any{"version": "2.5.8", "platforms": map[string]any{
		"darwin-aarch64":     map[string]string{"url": api + "623652157", "signature": "s"},
		"darwin-aarch64-app": map[string]string{"url": api + "623652157", "signature": "s"},
		"darwin-x86_64":      map[string]string{"url": "https://api.github.com/repos/evil/app/releases/assets/42", "signature": "s"},
	}}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.client().ReleaseMetadata(context.Background(), []Asset{mac, other, s.add("latest.json", b)})
	if err != nil {
		t.Fatal(err)
	}
	if m := got[mac.Name]; len(m.Conflicts) > 0 || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "macos", Arch: "arm64"}}) {
		t.Fatalf("REST URL not bound: %+v", m)
	}
	if _, ok := got[other.Name]; ok {
		t.Fatal("asset ID from another repo was trusted")
	}
}
