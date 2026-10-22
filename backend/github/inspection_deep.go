package github

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// DeepInspectorVersion invalidates persisted facts when parser or tool behavior changes.
const DeepInspectorVersion = "full-v3-launcher-7zip26.04-inno0.7"

const (
	deepMaxDownload = 2 << 30
	deepMaxStaged   = 4 << 30
	deepMaxPrefix   = 1 << 20
	deepMaxEntries  = 100000
	deepMaxDepth    = 6
)

// DeepInspector reads full public assets outside request handlers and never executes them.
// DownloadDir may retain source bytes; only validated, size-matched files are reused.
type DeepInspector struct {
	DownloadDir string
	SevenZip    string
	Inno        string
	client      *Client
}

type deepSession struct {
	ctx       context.Context
	inspector *DeepInspector
	dir       string
	staged    int64
	commands  int
}

// Inspect downloads one immutable asset, then inspects its application contents.
func (d *DeepInspector) Inspect(ctx context.Context, asset Asset) (ArtifactMetadata, error) {
	if asset.Size > deepMaxDownload {
		return deepUnknown("asset exceeds the 2 GiB full-download limit"), nil
	}
	filename, cleanup, err := d.download(ctx, asset)
	if err != nil {
		return ArtifactMetadata{}, err
	}
	defer cleanup()
	return d.InspectFile(ctx, asset, filename)
}

// InspectFile validates a local copy against the asset before opening any payloads.
func (d *DeepInspector) InspectFile(ctx context.Context, asset Asset, filename string) (ArtifactMetadata, error) {
	if _, ok := releaseDownloadURL(asset); !ok {
		return ArtifactMetadata{}, errors.New("deep inspection requires an uploaded public GitHub release asset")
	}
	info, err := os.Stat(filename)
	if err != nil {
		return ArtifactMetadata{}, err
	}
	if !info.Mode().IsRegular() || info.Size() != asset.Size || info.Size() > deepMaxDownload {
		return ArtifactMetadata{}, errors.New("local artifact does not match its declared size")
	}
	f, err := os.Open(filename)
	if err != nil {
		return ArtifactMetadata{}, err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, &deepContextReader{ctx: ctx, r: f})
	f.Close()
	if err != nil {
		return ArtifactMetadata{}, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if asset.Digest != "" && asset.Digest != "sha256:"+digest {
		return ArtifactMetadata{}, errors.New("artifact SHA-256 does not match GitHub metadata")
	}
	dir, err := os.MkdirTemp("", "yatko-inspect-")
	if err != nil {
		return ArtifactMetadata{}, err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	s := &deepSession{ctx: ctx, inspector: d, dir: dir}
	m, err := s.inspect(filename, asset.Name, 0)
	if err != nil {
		if errors.Is(err, errInspectLimit) {
			m = deepUnknown(err.Error())
			m.Status = "unsupported"
		} else {
			return m, err
		}
	}
	if strings.HasSuffix(strings.ToLower(asset.Name), ".appimage") {
		launcher, err := deepLocalHeaders(filename, asset.Name)
		if err != nil {
			return ArtifactMetadata{}, err
		}
		setAppImageLauncher(&launcher)
		m.LauncherArch = launcher.LauncherArch
	}
	for i := range m.Evidence {
		m.Evidence[i].Source = "deep-inspection"
	}
	deepNote(&m, "inspector.version", DeepInspectorVersion)
	deepNote(&m, "asset.sha256", digest)
	return m, nil
}

// inspect unwraps containers until it reaches main executable headers or package targets.
func (s *deepSession) inspect(filename, name string, depth int) (ArtifactMetadata, error) {
	if err := s.ctx.Err(); err != nil {
		return ArtifactMetadata{}, err
	}
	if depth > deepMaxDepth {
		return deepUnknown("container nesting exceeds six levels"), nil
	}
	if isPBZX(filename) {
		cpio, err := s.decodePBZX(filename)
		if err != nil {
			return ArtifactMetadata{}, err
		}
		return s.inspect(cpio, name+".cpio", depth+1)
	}
	if m, recognized, err := s.inspectBurn(filename, depth); recognized || err != nil {
		return m, err
	}
	if strings.HasSuffix(strings.ToLower(name), ".flatpak") {
		f, err := os.Open(filename)
		if err != nil {
			return ArtifactMetadata{}, err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return ArtifactMetadata{}, err
		}
		m := ArtifactMetadata{Status: "inspected", Role: "application"}
		if err := inspectFlatpakFile(f, info.Size(), &m); err != nil {
			return m, err
		}
		return m, nil
	}
	a, err := s.openArchive(filename, name)
	if err != nil {
		return ArtifactMetadata{}, err
	}
	a.depth = depth
	if a.kind == "PE" || a.kind == "" {
		m, err := deepLocalBinary(filename, name)
		if err != nil {
			return m, err
		}
		if slices.ContainsFunc(m.Evidence, func(e ArtifactEvidence) bool { return strings.HasSuffix(e.Field, "pe.machine") }) && !standalonePE(filename) {
			return deepUnknown("PE wrapper has no readable application payload"), nil
		}
		return m, nil
	}
	if m, ok, err := a.inspectMacApps(); ok || err != nil {
		return m, err
	}
	if m, ok, err := a.inspectMSIX(); ok || err != nil {
		return m, err
	}
	// Electron bundles can carry several CPU-specific app archives in one installer.
	var packages []deepEntry
	for _, e := range a.entries {
		base := strings.ToLower(filepath.Base(e.path))
		if !e.directory && (strings.HasPrefix(base, "app-") && hasAnySuffix(base, ".7z", ".zip")) {
			packages = append(packages, e)
		}
	}
	if len(packages) > 0 {
		var out ArtifactMetadata
		out.Status, out.Role = "inspected", "installer"
		for _, e := range packages {
			path, err := a.stage(e)
			if err != nil {
				return out, err
			}
			m, err := s.inspect(path, e.path, depth+1)
			if err != nil {
				return out, err
			}
			if !deepProven(m) {
				return m, nil
			}
			for _, t := range m.Targets {
				addTarget(&out, t)
			}
			out.Evidence = append(out.Evidence, m.Evidence...)
		}
		return out, nil
	}
	if m, ok, err := a.inspectExecutables(name); ok || err != nil {
		return m, err
	}
	if m, ok, err := a.inspectRuntimeLibraries(); ok || err != nil {
		return m, err
	}
	var nested []deepEntry
	for _, e := range a.entries {
		if e.directory || e.size == 0 || e.size > deepMaxDownload {
			continue
		}
		base := strings.ToLower(filepath.Base(e.path))
		if len(a.entries) == 1 || hasAnySuffix(base, ".cab", ".pkg", ".cpio", ".tar", ".7z", ".zip", ".nupkg", ".xz", ".gz", ".zst", ".pbzx") || base == "payload" {
			nested = append(nested, e)
		}
	}
	if len(nested) > 16 {
		return deepUnknown("more than sixteen candidate nested payloads"), nil
	}
	var results []ArtifactMetadata
	var unresolved *ArtifactMetadata
	for _, e := range nested {
		path, err := a.stage(e)
		if err != nil {
			return ArtifactMetadata{}, err
		}
		m, err := s.inspect(path, e.path, depth+1)
		if err != nil {
			return m, err
		}
		if deepProven(m) {
			results = append(results, m)
		} else {
			deepNote(&m, "container.entry", e.path)
			unresolved = &m
		}
	}
	if len(results) > 0 {
		return commonDeepTargets(results), nil
	}
	if unresolved != nil {
		return *unresolved, nil
	}
	m := deepUnknown("no main application executable or architecture-bound package metadata in " + a.kind)
	if strings.EqualFold(a.kind, "Nsis") {
		m = deepUnknown("NSIS container has no embedded application executable; application CPU requires its external payload")
		m.Role = "installer"
	}
	deepNote(&m, "container.format", a.kind)
	return m, nil
}

// standalonePE permits image padding or a terminal Authenticode certificate, not unknown overlays.
func standalonePE(filename string) bool {
	f, err := os.Open(filename)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	head := make([]byte, min(info.Size(), deepMaxPrefix))
	if _, err := f.ReadAt(head, 0); err != nil {
		return false
	}
	overlay := peOverlay(head, info.Size())
	if overlay <= 0 {
		return false
	}
	if overlay == info.Size() {
		return true
	}
	le := binary.LittleEndian
	pe := int64(le.Uint32(head[60:]))
	opt := pe + 24
	if opt+2 > int64(len(head)) {
		return false
	}
	dd := int64(96)
	if le.Uint16(head[opt:]) == 0x20b {
		dd = 112
	}
	cert := opt + dd + 4*8
	if cert+8 > int64(len(head)) {
		return false
	}
	offset, size := int64(le.Uint32(head[cert:])), int64(le.Uint32(head[cert+4:]))
	if offset < overlay || offset-overlay > 512 || size < 8 || offset+size != info.Size() {
		return false
	}
	padding := make([]byte, offset-overlay)
	_, err = f.ReadAt(padding, overlay)
	return err == nil && !slices.ContainsFunc(padding, func(b byte) bool { return b != 0 })
}

func deepLocalBinary(filename, name string) (ArtifactMetadata, error) {
	m, err := deepLocalHeaders(filename, name)
	if err != nil {
		return m, err
	}
	if m.Role != "application" {
		return deepUnknown("file is not a recognized application executable"), nil
	}
	return m, nil
}

func deepLocalHeaders(filename, name string) (ArtifactMetadata, error) {
	f, err := os.Open(filename)
	if err != nil {
		return ArtifactMetadata{}, err
	}
	defer f.Close()
	m := ArtifactMetadata{Role: "unknown", Status: "inspected"}
	found, err := sniffBinary(f, &m, name)
	if err != nil {
		return m, err
	}
	if !found {
		return deepUnknown("file has no recognized executable header"), nil
	}
	for _, e := range m.Evidence {
		if strings.Contains(e.Field, "pe.hybrid") && (strings.Contains(e.Value, "unreadable") || strings.Contains(e.Value, "unsupported")) {
			return deepUnknown("incomplete hybrid executable metadata: " + name), nil
		}
	}
	return m, nil
}

type deepContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *deepContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func deepProven(m ArtifactMetadata) bool {
	return !m.TargetsPartial && m.Status == "inspected" && len(m.Conflicts) == 0 && len(m.Targets) > 0 && !slices.ContainsFunc(m.Targets, func(t ArtifactTarget) bool {
		return t.Platform == "" || t.Arch == "" || strings.Contains(t.Arch, ":")
	})
}

func deepNote(m *ArtifactMetadata, field, value string) {
	m.Evidence = append(m.Evidence, ArtifactEvidence{Source: "deep-inspection", Field: field, Value: value})
}

func deepUnknown(reason string) ArtifactMetadata {
	m := ArtifactMetadata{Status: "unknown", Role: "unknown"}
	deepNote(&m, "inspection.reason", reason)
	return m
}

// commonDeepTargets requires candidate applications to agree, so a helper cannot add a CPU.
func commonDeepTargets(results []ArtifactMetadata) ArtifactMetadata {
	if len(results) == 0 {
		return deepUnknown("no application targets")
	}
	out := results[0]
	out.Targets = slices.Clone(out.Targets)
	for _, m := range results[1:] {
		out.TargetsPartial = out.TargetsPartial || m.TargetsPartial
		out.Targets = slices.DeleteFunc(out.Targets, func(t ArtifactTarget) bool { return !slices.Contains(m.Targets, t) })
		out.Evidence = append(out.Evidence, m.Evidence...)
	}
	if len(out.Targets) == 0 {
		return deepUnknown("candidate main applications have different CPU targets")
	}
	return out
}

// DeepDownloadName uses only a URL hash as a local filename, never untrusted asset paths.
func DeepDownloadName(asset Asset) string {
	h := sha256.Sum256([]byte(asset.BrowserDownloadURL))
	return fmt.Sprintf("%x.asset", h)
}
