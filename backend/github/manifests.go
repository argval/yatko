package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"
)

// Manifest bounds: cargo-dist manifests embed release notes; Electron update
// files are a few KB, so YAML gets a much smaller cap.
const (
	maxDistManifestBytes = 2 << 20
	maxUpdateYAMLBytes   = 256 << 10
	maxManifests         = 8
)

// electronUpdateFile matches electron-builder channel files: latest.yml
// (Windows, any arch), latest-mac.yml (macOS, any arch), latest-linux.yml
// (x64), and latest-linux-{arm64,arm,ia32}.yml. armv7l is an older spelling.
var electronUpdateFile = regexp.MustCompile(`^(?:latest|beta|alpha)(-mac|-linux(?:-(arm64|arm|armv7l|ia32))?)?\.yml$`)

// ReleaseMetadata reads publisher manifests uploaded with the release
// (cargo-dist dist-manifest.json, electron-builder latest*.yml) and returns
// facts keyed by exact existing asset Name. Manifest entries only corroborate
// assets already in the inventory: names must match exactly, and absolute
// URLs must equal an asset's BrowserDownloadURL. No URL is built or followed
// beyond the manifest assets themselves. Errors (download or malformed) are
// joined; facts from other manifests are still returned.
func (c *Client) ReleaseMetadata(ctx context.Context, assets []Asset) (map[string]ArtifactMetadata, error) {
	inv := inventory{byName: map[string]bool{}, byURL: map[string]string{}}
	for _, a := range assets {
		inv.byName[a.Name] = true
		inv.byURL[a.BrowserDownloadURL] = a.Name
	}
	out := map[string]ArtifactMetadata{}
	var errs []error
	read := 0
	for _, a := range assets {
		var limit int64
		var parse func([]byte) (map[string]ArtifactMetadata, error)
		if a.Name == "dist-manifest.json" {
			limit, parse = maxDistManifestBytes, func(b []byte) (map[string]ArtifactMetadata, error) { return parseDistManifest(a.Name, b, inv) }
		} else if g := electronUpdateFile.FindStringSubmatch(a.Name); g != nil {
			limit, parse = maxUpdateYAMLBytes, func(b []byte) (map[string]ArtifactMetadata, error) {
				return parseElectronUpdate(a.Name, b, electronTarget(g[1], g[2]), inv)
			}
		}
		if parse == nil || read == maxManifests {
			continue
		}
		read++
		self := ArtifactMetadata{Role: "metadata", Status: "inspected", Evidence: []ArtifactEvidence{{Source: a.Name, Field: "manifest", Value: a.Name}}}
		data, err := c.downloadManifest(ctx, a, limit)
		if err == nil {
			var facts map[string]ArtifactMetadata
			if facts, err = parse(data); err == nil {
				for name, fact := range facts {
					mergeManifestFact(out, name, fact)
				}
			}
		}
		if err != nil {
			self.Status = "failed"
			errs = append(errs, fmt.Errorf("%s: %w", a.Name, err))
		}
		mergeManifestFact(out, a.Name, self)
	}
	return out, errors.Join(errs...)
}

type inventory struct {
	byName map[string]bool
	byURL  map[string]string
}

// assetName resolves a manifest reference to an existing asset: an exact name,
// a percent-encoded bare name, or an absolute URL equal to an asset's download
// URL. Paths ("../x", "dir/x") and other URLs never match.
func (inv inventory) assetName(ref string) (string, bool) {
	if inv.byName[ref] {
		return ref, true
	}
	if name, ok := inv.byURL[ref]; ok {
		return name, true
	}
	if name, err := url.PathUnescape(ref); err == nil && !strings.ContainsAny(name, "/\\") && inv.byName[name] {
		return name, true
	}
	return "", false
}

func (c *Client) downloadManifest(ctx context.Context, a Asset, limit int64) ([]byte, error) {
	u, ok := releaseDownloadURL(a)
	if !ok {
		return nil, errors.New("not an uploaded GitHub release download")
	}
	if a.Size > limit {
		return nil, fmt.Errorf("manifest is %d bytes, limit %d", a.Size, limit)
	}
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	return c.openAsset(ctx, u, a.Size).fetch(0, a.Size, a.Size)
}

// mergeManifestFact combines facts for one asset from several manifests and
// records a conflict when they name different platforms or roles.
func mergeManifestFact(out map[string]ArtifactMetadata, name string, fact ArtifactMetadata) {
	have, ok := out[name]
	if !ok {
		out[name] = fact
		return
	}
	if p, q := manifestPlatforms(have), manifestPlatforms(fact); len(p) > 0 && len(q) > 0 && !slices.Equal(p, q) {
		have.Conflicts = append(have.Conflicts, fmt.Sprintf("manifests disagree on platform: %v vs %v", p, q))
	}
	if have.Role != fact.Role && have.Role != "unknown" && fact.Role != "unknown" {
		have.Conflicts = append(have.Conflicts, fmt.Sprintf("manifests disagree on role: %s vs %s", have.Role, fact.Role))
	} else if have.Role == "unknown" {
		have.Role = fact.Role
	}
	for _, t := range fact.Targets {
		addTarget(&have, t)
	}
	have.Evidence = append(have.Evidence, fact.Evidence...)
	have.Conflicts = append(have.Conflicts, fact.Conflicts...)
	out[name] = have
}

func manifestPlatforms(m ArtifactMetadata) []string {
	var ps []string
	for _, t := range m.Targets {
		if t.Platform != "" && !slices.Contains(ps, t.Platform) {
			ps = append(ps, t.Platform)
		}
	}
	slices.Sort(ps)
	return ps
}

// distManifest is the subset of cargo-dist's DistManifest schema used here.
// Every field is optional upstream; artifacts maps ArtifactId to Artifact, and
// an artifact without name has no file. Unknown kinds and fields are ignored.
type distManifest struct {
	DistVersion string `json:"dist_version"`
	Artifacts   map[string]struct {
		Name          *string  `json:"name"`
		Kind          string   `json:"kind"`
		TargetTriples []string `json:"target_triples"`
	} `json:"artifacts"`
}

// distKindRole maps cargo-dist artifact kinds. "updater" is a self-update
// helper, so it is excluded from application downloads.
var distKindRole = map[string]string{
	"executable-zip":      "application",
	"updater":             "metadata",
	"installer":           "installer",
	"symbols":             "debug",
	"source-tarball":      "source",
	"checksum":            "metadata",
	"unified-checksum":    "metadata",
	"sbom":                "metadata",
	"omnibor-artifact-id": "metadata",
}

func parseDistManifest(source string, data []byte, inv inventory) (map[string]ArtifactMetadata, error) {
	var doc distManifest
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedArtifact, err)
	}
	if doc.Artifacts == nil {
		return nil, fmt.Errorf("%w: dist manifest has no artifacts map", ErrMalformedArtifact)
	}
	out := map[string]ArtifactMetadata{}
	for _, art := range doc.Artifacts {
		if art.Name == nil {
			continue
		}
		name, ok := inv.assetName(*art.Name)
		if !ok {
			continue // published elsewhere (Homebrew, npm) or not uploaded
		}
		m := ArtifactMetadata{Role: "metadata", Status: "inspected"}
		if role := distKindRole[art.Kind]; role != "" {
			m.Role = role
		}
		m.Evidence = append(m.Evidence, ArtifactEvidence{Source: source, Field: "dist_version", Value: doc.DistVersion},
			ArtifactEvidence{Source: source, Field: "artifacts.kind", Value: art.Kind})
		for _, triple := range art.TargetTriples {
			m.Evidence = append(m.Evidence, ArtifactEvidence{Source: source, Field: "artifacts.target_triples", Value: triple})
			for _, t := range tripleTargets(triple) {
				addTarget(&m, t)
			}
		}
		mergeManifestFact(out, name, m)
	}
	return out, nil
}

// tripleTargets parses a Rust target triple (arch-vendor-os[-env]).
// cargo-dist adds -dynamic/-static to musl triples and universal2 for Mac.
func tripleTargets(triple string) []ArtifactTarget {
	arch, rest, ok := strings.Cut(triple, "-")
	if !ok {
		return nil
	}
	var t ArtifactTarget
	switch {
	case strings.Contains(rest, "apple-darwin"):
		t.Platform = "macos"
	case strings.HasPrefix(rest, "apple-ios"):
		t.Platform = "ios"
	case strings.Contains(rest, "windows"):
		t.Platform = "windows"
	case strings.Contains(rest, "linux-android"):
		t.Platform = "android"
	case strings.Contains(rest, "linux"):
		t.Platform = "linux"
		switch {
		case strings.HasSuffix(rest, "musl-static"):
			t.Libc = "static"
		case strings.Contains(rest, "musl"):
			t.Libc = "musl"
		case strings.Contains(rest, "gnu"):
			t.Libc = "gnu"
		}
	case strings.Contains(rest, "freebsd"):
		t.Platform = "freebsd"
	default:
		t.Platform = "target:" + rest
	}
	switch {
	case arch == "universal2" && t.Platform == "macos":
		a, b := t, t
		a.Arch, b.Arch = "amd64", "arm64"
		return []ArtifactTarget{a, b}
	case arch == "x86_64":
		t.Arch = "amd64"
	case arch == "aarch64", arch == "arm64":
		t.Arch = "arm64"
	case arch == "i686", arch == "i586", arch == "i386":
		t.Arch = "386"
	case strings.HasPrefix(arch, "arm"), strings.HasPrefix(arch, "thumbv7"):
		t.Arch = "arm"
	case strings.HasPrefix(arch, "riscv64"):
		t.Arch = "riscv64"
	default:
		t.Arch = arch
	}
	return []ArtifactTarget{t}
}

// electronUpdate is electron-builder's UpdateInfo. files[].url is a bare file
// name by default; path is the legacy single-file field. Windows packages
// (web installer payloads) are keyed by arch.
type electronUpdate struct {
	Version string `yaml:"version"`
	Files   []struct {
		URL string `yaml:"url"`
	} `yaml:"files"`
	Path     string `yaml:"path"`
	Packages map[string]struct {
		Path string `yaml:"path"`
	} `yaml:"packages"`
}

// electronTarget derives the target from the channel file suffix. Only Linux
// files carry an arch (x64 has no suffix); Windows and macOS files may list
// several arches, so arch stays unknown there.
func electronTarget(platform, arch string) ArtifactTarget {
	switch platform {
	case "":
		return ArtifactTarget{Platform: "windows"}
	case "-mac":
		return ArtifactTarget{Platform: "macos"}
	}
	t := ArtifactTarget{Platform: "linux", Arch: "amd64"}
	switch arch {
	case "arm64":
		t.Arch = "arm64"
	case "arm", "armv7l":
		t.Arch = "arm"
	case "ia32":
		t.Arch = "386"
	}
	return t
}

var electronPackageArch = map[string]string{"x64": "amd64", "ia32": "386", "arm64": "arm64"}

func parseElectronUpdate(source string, data []byte, target ArtifactTarget, inv inventory) (map[string]ArtifactMetadata, error) {
	// Update files need no YAML references. Reject them before decoding to bound
	// expansion work on untrusted release content.
	for _, tok := range lexer.Tokenize(string(data)) {
		if tok.Type == token.AnchorType || tok.Type == token.AliasType {
			return nil, fmt.Errorf("%w: YAML references are unsupported", ErrMalformedArtifact)
		}
	}
	var doc electronUpdate
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedArtifact, err)
	}
	if doc.Version == "" || (len(doc.Files) == 0 && doc.Path == "") {
		return nil, fmt.Errorf("%w: update info needs version and files", ErrMalformedArtifact)
	}
	out := map[string]ArtifactMetadata{}
	add := func(field, ref string, t ArtifactTarget) {
		name, ok := inv.assetName(ref)
		if !ok {
			return
		}
		m := ArtifactMetadata{Role: "unknown", Status: "inspected", Targets: []ArtifactTarget{t},
			Evidence: []ArtifactEvidence{{Source: source, Field: field, Value: ref}}}
		mergeManifestFact(out, name, m)
	}
	for _, f := range doc.Files {
		add("files.url", f.URL, target)
	}
	if doc.Path != "" {
		add("path", doc.Path, target)
	}
	for arch, p := range doc.Packages {
		t := target
		if a := electronPackageArch[arch]; a != "" && t.Platform == "windows" {
			t.Arch = a
		}
		add("packages."+arch+".path", p.Path, t)
	}
	return out, nil
}
