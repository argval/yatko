package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

type deepEntry struct {
	path        string
	size        int64
	directory   bool
	index       int
	destination string
	check       string // Inno Check: condition, e.g. "IsArm64()"
}

type deepArchive struct {
	session   *deepSession
	filename  string
	kind      string
	depth     int
	appName   string // Inno AppName; names the main executable
	entries   []deepEntry
	readEntry func(deepEntry, int64, int64) ([]byte, error)
}

type innoListing struct {
	Format  string `json:"format"`
	Version string `json:"version"`
	AppName string `json:"app_name"`
	Files   []struct {
		Index       int    `json:"index"`
		Path        string `json:"path"`
		Destination string `json:"destination"`
		Size        int64  `json:"size"`
		External    bool   `json:"external"`
		Check       string `json:"check"`
	} `json:"files"`
}

// openArchive asks pinned extractors for structured listings, never installer execution.
func (s *deepSession) openArchive(filename, name string) (*deepArchive, error) {
	a := &deepArchive{session: s, filename: filename}
	if strings.HasSuffix(strings.ToLower(name), ".exe") && s.inspector.Inno != "" {
		out, err := s.run(s.inspector.Inno, []string{"list", filename}, 8<<20)
		if err == nil {
			var listing innoListing
			if err := json.Unmarshal(out, &listing); err != nil {
				return nil, fmt.Errorf("decode Inno listing: %w", err)
			}
			if listing.Format != "inno" || len(listing.Files) > deepMaxEntries {
				return nil, errors.New("invalid or excessive Inno file listing")
			}
			a.kind = "Inno " + listing.Version
			a.appName = listing.AppName
			for _, f := range listing.Files {
				if !f.External && f.Size >= 0 {
					a.entries = append(a.entries, deepEntry{path: strings.ReplaceAll(f.Path, "\\", "/"), size: f.Size, index: f.Index, destination: f.Destination, check: f.Check})
				}
			}
			return a, nil
		}
		if s.ctx.Err() != nil {
			return nil, s.ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, err
		}
	}
	tool := s.inspector.SevenZip
	if tool == "" {
		tool = "7zz"
	}
	out, err := s.run(tool, []string{"l", "-slt", "-mmt=1", "-p-", "--", filename}, 32<<20)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, errInspectLimit) || s.ctx.Err() != nil {
			return nil, err
		}
		return a, nil // A standalone binary is not an archive.
	}
	a.kind, a.entries, err = parseDeepListing(out)
	return a, err
}

func parseDeepListing(out []byte) (string, []deepEntry, error) {
	text := strings.ReplaceAll(string(out), "\r\n", "\n")
	header, listing, ok := strings.Cut(text, "----------\n")
	if !ok {
		return "", nil, errors.New("extractor did not return a technical file listing")
	}
	kind := ""
	for _, line := range strings.Split(header, "\n") {
		if strings.HasPrefix(line, "Type = ") {
			kind = strings.TrimPrefix(line, "Type = ")
		}
	}
	var entries []deepEntry
	for _, block := range strings.Split(listing, "\n\n") {
		fields := map[string]string{}
		for _, line := range strings.Split(block, "\n") {
			key, value, found := strings.Cut(line, " = ")
			if !found {
				continue
			}
			if _, duplicate := fields[key]; duplicate {
				return "", nil, errors.New("ambiguous extractor record")
			}
			fields[key] = value
		}
		name := strings.ReplaceAll(fields["Path"], "\\", "/")
		if name == "" {
			continue
		}
		if strings.ContainsAny(name, "\x00\r\n") || fields["Symbolic Link"] != "" || fields["Hard Link"] != "" || strings.HasPrefix(fields["Mode"], "l") {
			continue
		}
		size := int64(-1) // Solid NSIS entries can omit decoded sizes.
		if fields["Size"] != "" {
			var err error
			if size, err = strconv.ParseInt(fields["Size"], 10, 64); err != nil || size < 0 {
				return "", nil, errors.New("invalid decoded entry size")
			}
		}
		entries = append(entries, deepEntry{path: name, size: size, directory: fields["Folder"] == "+" || strings.HasPrefix(fields["Mode"], "d") || fields["Type"] == "directory"})
		if len(entries) > deepMaxEntries {
			return "", nil, fmt.Errorf("%w: archive listing exceeds 100000 entries", errInspectLimit)
		}
	}
	return kind, entries, nil
}

func (a *deepArchive) read(e deepEntry, off, n int64) ([]byte, error) {
	if off < 0 || n < 0 || off > e.size || n > e.size-off || n > 16<<20 {
		return nil, errors.New("entry read exceeds its declared bounds")
	}
	if a.readEntry != nil {
		return a.readEntry(e, off, n)
	}
	if strings.HasPrefix(a.kind, "Inno ") {
		return a.session.run(a.session.inspector.Inno, []string{"read", a.filename, strconv.Itoa(e.index), strconv.FormatInt(n, 10), strconv.FormatInt(off, 10)}, n)
	}
	tool := a.session.inspector.SevenZip
	if tool == "" {
		tool = "7zz"
	}
	reader := &deepCapture{skip: off, limit: n}
	err := a.session.command(tool, []string{"x", "-so", "-spd", "-mmt=1", "-y", "-p-", "--", a.filename, e.path}, reader)
	if int64(reader.buf.Len()) == n {
		return reader.buf.Bytes(), nil // Prefix reads intentionally stop the extractor.
	}
	return reader.buf.Bytes(), err
}

// stage writes only generated filenames, with per-file and total decoded-byte limits.
func (a *deepArchive) stage(e deepEntry) (string, error) {
	if e.size == 0 || e.size > deepMaxDownload || (e.size > 0 && a.session.staged+e.size > deepMaxStaged) || a.session.staged >= deepMaxStaged {
		return "", errors.New("nested payload exceeds decoded disk budget")
	}
	f, err := os.CreateTemp(a.session.dir, "payload-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	limit := e.size
	if limit < 0 {
		limit = min(deepMaxDownload, deepMaxStaged-a.session.staged)
	}
	writer := &deepFileWriter{file: f, limit: limit}
	if strings.HasPrefix(a.kind, "Inno ") {
		if e.size > 16<<20 {
			return "", errors.New("Inno nested archive exceeds the 16 MiB streaming-read limit")
		}
		data, err := a.read(e, 0, e.size)
		if err != nil {
			return "", err
		}
		if _, err := writer.Write(data); err != nil {
			return "", err
		}
		a.session.staged += writer.written
		return f.Name(), nil
	}
	tool := a.session.inspector.SevenZip
	if tool == "" {
		tool = "7zz"
	}
	err = a.session.command(tool, []string{"x", "-so", "-spd", "-mmt=1", "-y", "-p-", "--", a.filename, e.path}, writer)
	if err != nil || (e.size >= 0 && writer.written != e.size) {
		return "", fmt.Errorf("extract nested payload %q: %w", e.path, errors.Join(err, io.ErrUnexpectedEOF))
	}
	a.session.staged += writer.written
	return f.Name(), nil
}

func (a *deepArchive) binary(e deepEntry) (ArtifactMetadata, error) {
	m, err := a.headers(e)
	if err != nil {
		return m, err
	}
	if m.Role != "application" {
		return deepUnknown("payload is not an application executable: " + e.path), nil
	}
	return m, nil
}

func (a *deepArchive) headers(e deepEntry) (ArtifactMetadata, error) {
	if e.size < 0 {
		filename, err := a.stage(e)
		if err != nil {
			return ArtifactMetadata{}, err
		}
		m, err := deepLocalHeaders(filename, e.path)
		deepNote(&m, "application.executable", e.path)
		return m, err
	}
	r := &deepEntryReader{archive: a, entry: e}
	m := ArtifactMetadata{Role: "unknown", Status: "inspected"}
	found, err := sniffBinary(r, &m, e.path)
	if r.err != nil {
		return m, r.err
	}
	if err != nil {
		return m, err
	}
	if !found {
		return deepUnknown("payload has no recognized executable header: " + e.path), nil
	}
	for _, evidence := range m.Evidence {
		if strings.Contains(evidence.Field, "pe.hybrid") && (strings.Contains(evidence.Value, "unreadable") || strings.Contains(evidence.Value, "unsupported")) {
			return deepUnknown("incomplete hybrid executable metadata: " + e.path), nil
		}
	}
	deepNote(&m, "application.executable", e.path)
	return m, nil
}

type deepEntryReader struct {
	archive *deepArchive
	entry   deepEntry
	spans   []span
	err     error
}

func (r *deepEntryReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= r.entry.size {
		return 0, io.EOF
	}
	n := min(int64(len(p)), r.entry.size-off)
	if n == 0 {
		return 0, nil
	}
	for _, cached := range r.spans {
		if off >= cached.off && off+n <= cached.off+int64(len(cached.data)) {
			copied := copy(p, cached.data[off-cached.off:])
			if copied < len(p) {
				return copied, io.EOF
			}
			return copied, nil
		}
	}
	if len(r.spans) >= 64 {
		r.err = errors.New("executable header exceeds 64 random reads")
		return 0, r.err
	}
	want := min(max(n, 64<<10), r.entry.size-off)
	b, err := r.archive.read(r.entry, off, want)
	if err == nil && int64(len(b)) != want {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		r.err = err
		return 0, err
	}
	r.spans = append(r.spans, span{off: off, data: b})
	return r.ReadAt(p, off)
}

func (a *deepArchive) inspectExecutables(name string) (metadata ArtifactMetadata, found bool, err error) {
	partial := false
	defer func() { metadata.TargetsPartial = metadata.TargetsPartial || partial }()
	var candidates []deepEntry
	for _, e := range a.entries {
		if e.directory || (e.size >= 0 && e.size < 64) || e.size > deepMaxDownload {
			continue
		}
		base := strings.ToLower(path.Base(e.path))
		if strings.Contains(strings.ToLower(e.destination), "{sys") || strings.Contains(strings.ToLower(e.destination), "{tmp}") ||
			strings.ContainsAny(e.path, "\x00\r\n") || strings.Contains(strings.ToLower(e.path), "/resources/") || strings.HasPrefix(e.path, "$") {
			continue
		}
		if hasAnyPrefix(base, "unins", "uninstall", "update", "crash", "chrome_crash", "7z", "elevate", "notification_helper", "vc_redist", "vcredist", "dxsetup", "squirrel") || strings.HasSuffix(base, "_executionstub.exe") {
			continue
		}
		if strings.HasSuffix(base, ".dll") || strings.HasSuffix(base, ".so") || strings.Contains(e.path, ".framework/") {
			continue
		}
		windows := strings.HasSuffix(base, ".exe")
		unix := !strings.Contains(base, ".") && (strings.Contains(e.path, "/bin/") || strings.HasPrefix(e.path, "bin/") || !strings.Contains(path.Clean(e.path), "/"))
		if !windows && !unix {
			continue
		}
		candidates = append(candidates, e)
	}
	if len(candidates) == 0 {
		return ArtifactMetadata{}, false, nil
	}
	slices.SortStableFunc(candidates, func(x, y deepEntry) int { return strings.Count(x.path, "/") - strings.Count(y.path, "/") })
	var results []ArtifactMetadata
	var gates, dests, names []string
	resultDepth := -1
	for i, e := range candidates {
		if i >= 64 {
			partial = true
			break
		}
		depth := strings.Count(e.path, "/")
		if resultDepth >= 0 && depth > resultDepth {
			break
		}
		m, err := a.binary(e)
		if err != nil {
			return m, true, err
		}
		if m.Role == "application" && len(m.Targets) > 0 {
			for i := range m.Targets {
				if m.Targets[i].Platform == "" && strings.HasPrefix(a.kind, "Inno ") {
					m.Targets[i].Platform = "windows"
				}
			}
			results = append(results, m)
			gates = append(gates, e.check)
			dests = append(dests, strings.ToLower(e.destination))
			names = append(names, path.Base(e.path))
			resultDepth = depth
		}
	}
	if len(results) == 0 {
		return ArtifactMetadata{}, false, nil
	}
	if strings.HasPrefix(a.kind, "Inno ") {
		if m, ok := cpuGatedTargets(results, gates, dests); ok {
			return m, true, nil
		}
		if m := commonDeepTargets(results); !deepProven(m) && a.appName != "" {
			// Helpers (exiftool, ffmpeg) may disagree with the app; the Inno
			// AppName names the main executable (ShareX -> ShareX.exe).
			var named []ArtifactMetadata
			for i, n := range names {
				if strings.EqualFold(n, a.appName+".exe") {
					named = append(named, results[i])
				}
			}
			if len(named) > 0 {
				m = commonDeepTargets(named)
				deepNote(&m, "inno.app_name", a.appName)
				return m, true, nil
			}
		}
	}
	return commonDeepTargets(results), true, nil
}

// innoCPUGates maps Inno Setup Check: functions that select files by the
// installing machine's CPU to the CPUs they admit (jrsoftware.org isfunc docs).
var innoCPUGates = map[string][]string{
	"isarm64()":            {"arm64"},
	"notisarm64()":         {"amd64", "386"},
	"isx64compatible()":    {"amd64"},
	"isx64os()":            {"amd64"},
	"isx64()":              {"amd64"},
	"isx86compatible()":    {"386"},
	"isx86os()":            {"386"},
	"isx86()":              {"386"},
	"notisx64compatible()": {"386", "arm64"},
}

// cpuGatedTargets accepts candidates that disagree on CPU only when the
// installer itself picks one copy per CPU. For each install destination, every
// copy carries a CPU Check that admits its own target and no two checks admit
// the same CPU; every destination must then cover the same CPUs. Ollama ships
// "not IsArm64()" x64 and "IsArm64()" arm64 copies of two programs, so the
// installer runs natively on both and the targets are the union.
func cpuGatedTargets(results []ArtifactMetadata, gates, dests []string) (ArtifactMetadata, bool) {
	if len(results) < 2 {
		return ArtifactMetadata{}, false
	}
	type program struct {
		cpus    map[string]bool
		targets []ArtifactTarget
	}
	programs := map[string]*program{}
	out := ArtifactMetadata{Status: "inspected", Role: "application"}
	for i, m := range results {
		admitted, ok := innoCPUGates[strings.ToLower(strings.Join(strings.Fields(gates[i]), ""))]
		if !ok || dests[i] == "" {
			return ArtifactMetadata{}, false
		}
		p := programs[dests[i]]
		if p == nil {
			p = &program{cpus: map[string]bool{}}
			programs[dests[i]] = p
		}
		for _, cpu := range admitted {
			if p.cpus[cpu] {
				return ArtifactMetadata{}, false // two copies of one program on one CPU
			}
			p.cpus[cpu] = true
		}
		for _, t := range m.Targets {
			if !slices.Contains(admitted, t.Arch) {
				return ArtifactMetadata{}, false // the check does not match the binary
			}
			if !slices.Contains(p.targets, t) {
				p.targets = append(p.targets, t)
			}
		}
		out.Evidence = append(out.Evidence, m.Evidence...)
		deepNote(&out, "inno.check", dests[i]+": "+gates[i])
	}
	for _, p := range programs {
		if out.Targets == nil {
			out.Targets = p.targets
			continue
		}
		if len(p.targets) != len(out.Targets) || slices.ContainsFunc(p.targets, func(t ArtifactTarget) bool { return !slices.Contains(out.Targets, t) }) {
			return ArtifactMetadata{}, false // programs disagree on the CPUs they cover
		}
	}
	return out, true
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

type deepCapture struct {
	skip      int64
	limit     int64
	buf       bytes.Buffer
	truncated bool
}

func (w *deepCapture) Write(p []byte) (int, error) {
	original := len(p)
	if w.skip > 0 {
		n := min(int64(len(p)), w.skip)
		w.skip -= n
		p = p[n:]
	}
	if int64(w.buf.Len())+int64(len(p)) > w.limit {
		w.buf.Write(p[:w.limit-int64(w.buf.Len())])
		w.truncated = true
		return original, errInspectLimit
	}
	w.buf.Write(p)
	return original, nil
}

type deepFileWriter struct {
	file      *os.File
	limit     int64
	written   int64
	truncated bool
}

func (w *deepFileWriter) Write(p []byte) (int, error) {
	if w.written+int64(len(p)) > w.limit {
		w.truncated = true
		return 0, errInspectLimit
	}
	n, err := w.file.Write(p)
	w.written += int64(n)
	return n, err
}

func (s *deepSession) run(tool string, args []string, limit int64) ([]byte, error) {
	w := &deepCapture{limit: limit}
	err := s.command(tool, args, w)
	return w.buf.Bytes(), err
}

// command limits child CPU, memory and output. All user-controlled names stay argv data.
func (s *deepSession) command(tool string, args []string, stdout io.Writer) error {
	s.commands++
	if s.commands > 256 {
		return errors.New("inspection exceeds 256 extractor commands")
	}
	resolved, err := exec.LookPath(tool)
	if err != nil {
		return err
	}
	script := `ulimit -t 120; ulimit -v 1572864; exec "$@"`
	if runtime.GOOS == "darwin" {
		script = `ulimit -t 120; ulimit -d 1572864; exec "$@"`
	}
	command := exec.CommandContext(s.ctx, "/bin/sh", append([]string{"-c", script, "extractor", resolved}, args...)...)
	command.Dir = s.dir
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "LC_ALL=C.UTF-8", "HOME=" + s.dir, "TMPDIR=" + s.dir}
	command.Stdout = stdout
	stderr := &deepCapture{limit: 64 << 10}
	command.Stderr = stderr
	command.WaitDelay = 2 * time.Second
	err = command.Run()
	if capture, ok := stdout.(*deepCapture); ok && capture.truncated {
		return fmt.Errorf("%w: extractor output exceeds its byte limit", errInspectLimit)
	}
	if writer, ok := stdout.(*deepFileWriter); ok && writer.truncated {
		return fmt.Errorf("%w: decoded payload exceeds its byte limit", errInspectLimit)
	}
	if err != nil {
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		return fmt.Errorf("%s: %w: %s", filepath.Base(tool), err, strings.TrimSpace(stderr.buf.String()))
	}
	return nil
}
