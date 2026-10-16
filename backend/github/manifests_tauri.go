package github

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// parseTauriUpdate reads the static updater schema documented by Tauri.
// Only references to uploaded assets in this release can contribute target facts.
func parseTauriUpdate(source string, data []byte, inv inventory) (map[string]ArtifactMetadata, error) {
	var doc struct {
		Version   string `json:"version"`
		Platforms map[string]struct {
			URL       string `json:"url"`
			Signature string `json:"signature"`
		} `json:"platforms"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedArtifact, err)
	}
	if strings.TrimSpace(doc.Version) == "" || len(doc.Platforms) == 0 {
		return nil, fmt.Errorf("%w: Tauri update needs version and platforms", ErrMalformedArtifact)
	}
	out := map[string]ArtifactMetadata{}
	for _, key := range slices.Sorted(maps.Keys(doc.Platforms)) {
		entry := doc.Platforms[key]
		if strings.TrimSpace(entry.Signature) == "" || entry.URL == "" {
			return nil, fmt.Errorf("%w: Tauri platform needs URL and signature", ErrMalformedArtifact)
		}
		name, ok := inv.assetName(entry.URL)
		if !ok {
			continue
		}
		// Custom target names are opaque. Only the documented default keys carry
		// OS/CPU meaning; unknown keys must not become wildcard targets.
		target := tauriTarget(key)
		m := ArtifactMetadata{Role: "application", Status: "inspected", Targets: []ArtifactTarget{target}, Evidence: []ArtifactEvidence{
			{Source: source, Field: "tauri.version", Value: doc.Version},
			{Source: source, Field: "platforms.target", Value: key},
			{Source: source, Field: "platforms.url", Value: entry.URL},
		}}
		mergeManifestFact(out, name, m)
	}
	return out, nil
}

// tauriTarget preserves unsupported/custom target strings as explicit constraints.
// Tauri v2 also emits OS-ARCH-BUNDLE keys (darwin-aarch64-app, windows-x86_64-nsis).
func tauriTarget(key string) ArtifactTarget {
	base := key
	if i := strings.LastIndexByte(key, '-'); i > 0 && slices.Contains([]string{"app", "appimage", "deb", "rpm", "msi", "nsis"}, key[i+1:]) {
		base = key[:i]
	}
	os, arch, ok := strings.Cut(base, "-")
	if !ok {
		return ArtifactTarget{Platform: "tauri-target:" + key}
	}
	platforms := map[string]string{"darwin": "macos", "windows": "windows", "linux": "linux"}
	cpus := map[string]string{"x86_64": "amd64", "aarch64": "arm64", "i686": "386", "armv7": "arm"}
	if platforms[os] == "" || cpus[arch] == "" {
		return ArtifactTarget{Platform: "tauri-target:" + key}
	}
	return ArtifactTarget{Platform: platforms[os], Arch: cpus[arch]}
}
