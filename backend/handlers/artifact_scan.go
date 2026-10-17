package handlers

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/argval/yatko/artifactscan"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

const scanRequestBudget = 250 * time.Millisecond

// scanArtifacts reads all saved evidence and queues missing inspections within one small budget.
// Full downloads run only in a separate worker; unavailable storage keeps the current fast path.
func (h *RedirectHandler) scanArtifacts(parent context.Context, assets []github.Asset) map[string]artifactscan.Record {
	if h.ScanStore == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, scanRequestBudget)
	defer cancel()
	records, err := h.ScanStore.LookupMany(ctx, assets)
	if err != nil {
		return records
	}
	for _, asset := range assets {
		record, found := records[asset.BrowserDownloadURL]
		if found && artifactIdentity(record.Asset) == artifactIdentity(asset) || !needsDeepInspection(asset) {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		record, err = h.ScanStore.Enqueue(ctx, asset)
		if err != nil {
			break
		}
		records[asset.BrowserDownloadURL] = record
	}
	return records
}

// completeScanTargets requires observed OS and CPU facts rather than filename-filled targets.
func completeScanTargets(metadata github.ArtifactMetadata) bool {
	return len(metadata.Targets) > 0 && !slices.ContainsFunc(metadata.Targets, func(target github.ArtifactTarget) bool {
		return target.Platform == "" || target.Arch == ""
	})
}

// authoritativeScanMetadata includes proof of an unusable file even without executable targets.
func authoritativeScanMetadata(metadata github.ArtifactMetadata) bool {
	if metadata.Status == "failed" || completeScanTargets(metadata) {
		return true
	}
	return metadata.Status == "inspected" && (metadata.Role == "source" || metadata.Role == "metadata" || metadata.Role == "debug")
}

// appendScanEvidence keeps fast facts when full inspection could not identify the payload.
func appendScanEvidence(asset *github.Asset, metadata github.ArtifactMetadata) {
	observed := github.ArtifactMetadata{Role: "unknown"}
	if asset.Artifact != nil {
		observed = *asset.Artifact
		observed.Evidence = slices.Clone(observed.Evidence)
	}
	for _, evidence := range metadata.Evidence {
		if !slices.Contains(observed.Evidence, evidence) {
			observed.Evidence = append(observed.Evidence, evidence)
		}
	}
	asset.Artifact = &observed
}

// needsDeepInspection includes named installers because their launcher's CPU can differ from the app.
// Archives need it until a manifest or content inspection proves both OS and CPU.
func needsDeepInspection(asset github.Asset) bool {
	parsed, err := url.Parse(asset.BrowserDownloadURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil ||
		!strings.Contains(parsed.Path, "/releases/download/") || asset.Size <= 0 ||
		(asset.State != "" && asset.State != "uploaded") {
		return false
	}
	facts := picker.FactsForAsset(asset)
	if facts.Role == "source" || facts.Role == "metadata" || facts.Role == "debug" || facts.NonNative {
		return false
	}
	desktop := false
	for _, platform := range facts.Platforms {
		if platform == picker.Windows || platform == picker.MacOS || platform == picker.Linux {
			desktop = true
		}
	}
	if len(facts.Platforms) > 0 && !desktop {
		return false
	}
	name := strings.ToLower(asset.Name)
	if !desktop && (strings.Contains(name, "firmware") || strings.HasPrefix(name, "lineage-") || strings.Contains(name, "factory-image")) {
		return false
	}
	for _, suffix := range []string{".dmg", ".exe", ".msi", ".pkg", ".msix", ".msixbundle", ".appx", ".appxbundle", ".flatpak"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	if asset.Artifact != nil && asset.Artifact.Status == "inspected" && completeScanTargets(*asset.Artifact) &&
		slices.ContainsFunc(asset.Artifact.Evidence, func(evidence github.ArtifactEvidence) bool {
			return evidence.Source != "" && evidence.Source != "filename"
		}) {
		return false
	}
	for _, suffix := range []string{".zip", ".7z", ".tar", ".gz", ".tgz", ".xz", ".txz", ".bz2", ".tbz", ".tbz2", ".zst", ".tzst", ".rar", ".deb", ".rpm", ".appimage"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
