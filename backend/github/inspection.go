package github

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Inspection bounds. One InspectArtifact call gets one deadline, a fixed
// number of HTTP range requests, and a downloaded-byte budget. Decompressed
// reads have their own cap so archives cannot inflate without limit.
const (
	inspectTimeout     = 5 * time.Second
	inspectMaxRequests = 6
	inspectMaxBytes    = 4 << 20
	inspectSpan        = 256 << 10 // minimum bytes fetched on a cache miss
	inspectHeadBytes   = 64 << 10  // bytes of an archive entry passed to header parsers
	inspectMaxInflated = 8 << 20
	inspectMaxBinaries = 2 // executables sniffed inside one archive
	maxDownloadHops    = 3
)

// ErrMalformedArtifact marks content that claims a known format (magic bytes,
// archive signature, manifest file name) but does not parse as that format.
var ErrMalformedArtifact = errors.New("malformed artifact")

var errInspectLimit = errors.New("artifact inspection limit reached")

// downloadHosts are the only hosts a release download may start on or
// redirect to. GitHub serves asset bytes from its objects CDN.
var downloadHosts = map[string]bool{
	"github.com":                            true,
	"objects.githubusercontent.com":         true,
	"release-assets.githubusercontent.com":  true,
	"github-releases.githubusercontent.com": true,
}

// InspectArtifact reads bounded byte ranges of a release asset and reports
// what its contents prove: executable headers (ELF, PE, Mach-O, fat Mach-O),
// ZIP/tar listings and the first entries' headers, and .deb control data.
// Nothing is executed or written to disk. Status is "unsupported" when the
// asset or format is not inspected (no error), "inspected" when parsed (Targets
// may still be empty: missing evidence), and "failed" with a non-nil error for
// transport errors or malformed content (errors.Is ErrMalformedArtifact).
// A Target with an empty Platform means the CPU is known but the OS is not.
func (c *Client) InspectArtifact(ctx context.Context, asset Asset) (ArtifactMetadata, error) {
	m := ArtifactMetadata{Role: "unknown", Status: "unsupported"}
	u, ok := releaseDownloadURL(asset)
	if !ok {
		note(&m, "", "inspection.skipped", "not an uploaded GitHub release download")
		return m, nil
	}
	name := strings.ToLower(asset.Name)
	var inspect func(*remoteFile, *ArtifactMetadata) error
	switch {
	case strings.HasSuffix(name, ".zip"):
		inspect = inspectZIP
	case hasAnySuffix(name, ".tar.gz", ".tgz"):
		inspect = tarInspector(func(r io.Reader) (io.Reader, error) { return gzip.NewReader(r) })
	case hasAnySuffix(name, ".tar.bz2", ".tbz2", ".tbz"):
		inspect = tarInspector(func(r io.Reader) (io.Reader, error) { return bzip2.NewReader(r), nil })
	case strings.HasSuffix(name, ".tar"):
		inspect = tarInspector(func(r io.Reader) (io.Reader, error) { return r, nil })
	case strings.HasSuffix(name, ".gz"):
		inspect = inspectGzip
	case strings.HasSuffix(name, ".deb"):
		inspect = inspectDeb
	case sniffable(name):
		inspect = inspectBinary
	default:
		note(&m, "", "inspection.skipped", "unsupported format "+path.Ext(name))
		return m, nil
	}
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	f := c.openAsset(ctx, u, asset.Size)
	err := inspect(f, &m)
	switch {
	case f.err != nil && !errors.Is(f.err, errInspectLimit):
		m.Status = "failed"
		return m, f.err
	case err != nil && errors.Is(f.err, errInspectLimit):
		// The limit was hit before any structure parsed: too large, not broken.
		note(&m, "", "inspection.skipped", "exceeds inspection limits")
		return m, nil
	case err != nil:
		m.Status = "failed"
		if !errors.Is(err, ErrMalformedArtifact) {
			err = fmt.Errorf("%w: %v", ErrMalformedArtifact, err)
		}
		return m, err
	}
	m.Status = "inspected"
	return m, nil
}

// releaseDownloadURL accepts only uploaded assets served from
// https://github.com/{owner}/{repo}/releases/download/...
func releaseDownloadURL(asset Asset) (string, bool) {
	u, err := url.Parse(asset.BrowserDownloadURL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil ||
		!strings.Contains(u.Path, "/releases/download/") || asset.Size <= 0 ||
		(asset.State != "" && asset.State != "uploaded") {
		return "", false
	}
	return u.String(), true
}

// openAsset returns a lazily fetched view of one release asset. Requests go
// through c.httpClient's transport but never carry the API token, and every
// redirect must stay on a known GitHub download host.
func (c *Client) openAsset(ctx context.Context, rawURL string, size int64) *remoteFile {
	hc := *c.httpClient
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxDownloadHops {
			return errors.New("too many download redirects")
		}
		if req.URL.Scheme != "https" || req.URL.User != nil || !downloadHosts[req.URL.Host] {
			return fmt.Errorf("download redirect to %q refused", req.URL.Host)
		}
		req.Header.Del("Authorization")
		return nil
	}
	return &remoteFile{ctx: ctx, client: &hc, url: rawURL, size: size}
}

// remoteFile is an io.ReaderAt over HTTP range requests. Fetched spans are
// cached; a miss fetches at least inspectSpan bytes. The request and byte
// budgets are enforced here, and the first fetch error is kept in err.
type remoteFile struct {
	ctx      context.Context
	client   *http.Client
	url      string
	size     int64
	spans    []span
	requests int
	bytes    int64
	err      error
}

type span struct {
	off  int64
	data []byte
}

func (f *remoteFile) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= f.size {
		return 0, io.EOF
	}
	want := min(int64(len(p)), f.size-off)
	data := f.cached(off, want)
	if data == nil {
		var err error
		if data, err = f.fetch(off, want, max(want, inspectSpan)); err != nil {
			return 0, err
		}
	}
	n := copy(p, data)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *remoteFile) cached(off, n int64) []byte {
	for _, s := range f.spans {
		if off >= s.off && off+n <= s.off+int64(len(s.data)) {
			return s.data[off-s.off:]
		}
	}
	return nil
}

// fetch downloads bytes [off, off+n), shrinking n to the remaining budget but
// never below need. The response must be an exact 206, or a 200 when off is 0
// (only the first n bytes are read).
func (f *remoteFile) fetch(off, need, n int64) ([]byte, error) {
	n = min(n, f.size-off, inspectMaxBytes-f.bytes)
	if f.requests >= inspectMaxRequests || n < need {
		return nil, f.fail(errInspectLimit)
	}
	f.requests++
	f.bytes += n
	req, err := http.NewRequestWithContext(f.ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, f.fail(err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+n-1))
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "Yatko/1.0")
	// Download requests must not carry the GitHub API token.
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, f.fail(err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusPartialContent:
		if got, want := resp.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-%d/%d", off, off+n-1, f.size); got != want {
			return nil, f.fail(fmt.Errorf("download range: Content-Range %q, want %q", got, want))
		}
	case resp.StatusCode == http.StatusOK && off == 0:
	default:
		return nil, f.fail(fmt.Errorf("download range: HTTP %d", resp.StatusCode))
	}
	limit := n
	if resp.StatusCode == http.StatusPartialContent {
		limit++ // detect an over-long body
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, f.fail(err)
	}
	if int64(len(data)) != n {
		return nil, f.fail(io.ErrUnexpectedEOF)
	}
	f.spans = append(f.spans, span{off, data})
	return data, nil
}

func (f *remoteFile) fail(err error) error {
	if f.err == nil {
		f.err = err
	}
	return err
}

// inspectBinary sniffs a standalone asset's header.
func inspectBinary(f *remoteFile, m *ArtifactMetadata) error {
	return sniffOrNote(f, m, "")
}

func sniffOrNote(r io.ReaderAt, m *ArtifactMetadata, at string) error {
	found, err := sniffBinary(r, m, at)
	if err == nil && !found {
		note(m, at, "format", "unrecognized")
	}
	return err
}

func inspectZIP(f *remoteFile, m *ArtifactMetadata) error {
	// Prefetch the tail so the central directory usually costs one request.
	n := min(f.size, archiveTailLimit)
	if _, err := f.fetch(f.size-n, n, n); err != nil {
		return err
	}
	r, err := zip.NewReader(f, f.size)
	if err != nil {
		return err
	}
	names := zipFileNames(r)
	archiveStructure(m, "zip", names)
	var candidates []*zip.File
	for _, zf := range r.File {
		if binaryCandidate(zf.Name, zf.Mode(), int64(zf.UncompressedSize64)) {
			candidates = append(candidates, zf)
		}
	}
	// A bundle's main executable says more than helpers under Frameworks/.
	slices.SortStableFunc(candidates, func(a, b *zip.File) int {
		return bundleRank(a.Name) - bundleRank(b.Name)
	})
	sniffed := 0
	bundle := macAppBundle(names)
	for _, zf := range candidates {
		if sniffed == inspectMaxBinaries {
			break
		}
		rc, err := zf.Open()
		if err != nil {
			note(m, "zip:"+zf.Name, "entry", err.Error())
			continue
		}
		head, err := io.ReadAll(io.LimitReader(rc, inspectHeadBytes))
		rc.Close()
		if f.err != nil {
			break // budget or transport; InspectArtifact reports transport errors
		}
		if err != nil && len(head) == 0 {
			note(m, "zip:"+zf.Name, "entry", err.Error())
			continue
		}
		sniffed++
		if err := inspectArchiveEntry(head, m, "zip:"+zf.Name, zf.Name, bundle); err != nil {
			return err
		}
	}
	return nil
}

// tarInspector lists tar entries through a decompressor and sniffs the first
// executable-mode entries. Large archives stop at the inflate or fetch budget.
func tarInspector(decompress func(io.Reader) (io.Reader, error)) func(*remoteFile, *ArtifactMetadata) error {
	return func(f *remoteFile, m *ArtifactMetadata) error {
		zr, err := decompress(io.NewSectionReader(f, 0, f.size))
		if err != nil {
			return err
		}
		tr := tar.NewReader(&capReader{r: zr, n: inspectMaxInflated})
		var names []string
		var entries []struct {
			name string
			head []byte
		}
		sniffed, partial := 0, false
		for sniffed < inspectMaxBinaries {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				if f.err != nil || errors.Is(err, errInspectLimit) {
					partial = true
					break
				}
				return fmt.Errorf("%w: tar: %v", ErrMalformedArtifact, err)
			}
			if h.Typeflag != tar.TypeReg {
				continue
			}
			name := strings.TrimPrefix(h.Name, "./")
			names = append(names, name)
			if !binaryCandidate(name, h.FileInfo().Mode(), h.Size) {
				continue
			}
			head, err := io.ReadAll(io.LimitReader(tr, inspectHeadBytes))
			if err != nil {
				partial = true
				break
			}
			sniffed++
			entries = append(entries, struct {
				name string
				head []byte
			}{name, head})
		}
		if partial {
			note(m, "", "tar.listing", "partial")
		}
		archiveStructure(m, "tar", names)
		bundle := macAppBundle(names)
		for _, entry := range entries {
			if err := inspectArchiveEntry(entry.head, m, "tar:"+entry.name, entry.name, bundle); err != nil {
				return err
			}
		}
		return nil
	}
}

// capReader returns errInspectLimit (not EOF) once n bytes are read, so a
// truncated listing is never mistaken for a malformed archive.
type capReader struct {
	r io.Reader
	n int64
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.n <= 0 {
		return 0, errInspectLimit
	}
	p = p[:min(int64(len(p)), c.n)]
	n, err := c.r.Read(p)
	c.n -= int64(n)
	return n, err
}

// inspectGzip sniffs the decompressed head of a single gzipped file.
func inspectGzip(f *remoteFile, m *ArtifactMetadata) error {
	zr, err := gzip.NewReader(io.NewSectionReader(f, 0, f.size))
	if err != nil {
		return err
	}
	head, err := io.ReadAll(io.LimitReader(zr, inspectHeadBytes))
	if err != nil && len(head) == 0 {
		return err
	}
	return sniffOrNote(bytes.NewReader(head), m, "gzip")
}

// inspectDeb walks the ar container and reads Architecture from the control
// file. Debian packages target Linux unless the architecture names another
// kernel (kfreebsd-*, hurd-*), which keeps its explicit kernel target.
func inspectDeb(f *remoteFile, m *ArtifactMetadata) error {
	magic := make([]byte, 8)
	if _, err := f.ReadAt(magic, 0); err != nil || string(magic) != "!<arch>\n" {
		return fmt.Errorf("%w: missing ar signature", ErrMalformedArtifact)
	}
	off := int64(8)
	for i := 0; i < 8 && off+60 <= f.size; i++ {
		h := make([]byte, 60)
		if _, err := f.ReadAt(h, off); err != nil {
			return err
		}
		size, err := strconv.ParseInt(strings.TrimSpace(string(h[48:58])), 10, 64)
		if string(h[58:60]) != "`\n" || err != nil || size < 0 || off+60+size > f.size {
			return fmt.Errorf("%w: bad ar member header", ErrMalformedArtifact)
		}
		member := strings.TrimSuffix(strings.TrimSpace(string(h[:16])), "/")
		body := io.NewSectionReader(f, off+60, size)
		switch {
		case i == 0 && member != "debian-binary":
			return fmt.Errorf("%w: first ar member is %q", ErrMalformedArtifact, member)
		case i == 0:
			m.Role = "installer"
			note(m, "", "deb.format", "debian-binary")
		case strings.HasPrefix(member, "control.tar"):
			return debControl(body, strings.TrimPrefix(member, "control.tar"), m)
		}
		off += 60 + size + size%2
	}
	note(m, "", "deb.control", "missing")
	return nil
}

func debControl(r io.Reader, compression string, m *ArtifactMetadata) error {
	switch compression {
	case ".gz":
		zr, err := gzip.NewReader(r)
		if err != nil {
			return err
		}
		r = zr
	case "":
	default:
		// ponytail: xz/zstd need new deps; filename evidence covers these.
		note(m, "", "deb.control_compression", strings.TrimPrefix(compression, "."))
		return nil
	}
	tr := tar.NewReader(&capReader{r: r, n: inspectMaxInflated})
	for {
		h, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				note(m, "", "deb.control", "missing")
				return nil
			}
			return err
		}
		if strings.TrimPrefix(h.Name, "./") != "control" {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, inspectHeadBytes))
		if err != nil {
			return err
		}
		for line := range strings.Lines(string(data)) {
			key, value, ok := strings.Cut(line, ":")
			if !ok || key != "Architecture" {
				continue
			}
			arch := strings.TrimSpace(value)
			note(m, "", "deb.architecture", arch)
			target := ArtifactTarget{Platform: "linux"}
			if kernel, cpu, ok := strings.Cut(arch, "-"); ok {
				target.Platform, target.Arch = kernel, debArch(cpu)
			} else if arch != "all" {
				target.Arch = debArch(arch)
			}
			addTarget(m, target)
			return nil
		}
		note(m, "", "deb.architecture", "missing")
		return nil
	}
}

func debArch(a string) string {
	switch a {
	case "armhf", "armel":
		return "arm"
	case "i386":
		return "386"
	}
	return a // amd64, arm64, riscv64 already match; others stay raw
}

// archiveStructure records signals visible from an archive listing alone:
// a macOS .app bundle (platform known, CPU unknown) or debug-symbols-only content.
func archiveStructure(m *ArtifactMetadata, kind string, names []string) {
	if bundle := macAppBundle(names); bundle != "" {
		note(m, "", kind+".app_bundle", bundle)
		addTarget(m, ArtifactTarget{Platform: "macos"})
		observeRole(m, "application")
	}
	debug := len(names) > 0
	for _, name := range names {
		lower := strings.ToLower(name)
		if !strings.Contains(lower, ".dsym/") && !hasAnySuffix(lower, ".pdb", ".debug", ".dbg") {
			debug = false
			break
		}
	}
	if debug {
		note(m, "", kind+".contents", "debug symbols only")
		observeRole(m, "debug")
	}
}

// observeRole keeps a proven application above its bundled helper files.
func observeRole(m *ArtifactMetadata, role string) {
	rank := map[string]int{"application": 4, "installer": 4, "debug": 2, "metadata": 1}
	if rank[role] > rank[m.Role] {
		m.Role = role
	}
}

// inspectArchiveEntry combines headers with structure only for the same app entry.
func inspectArchiveEntry(head []byte, m *ArtifactMetadata, at, name, bundle string) error {
	observed := ArtifactMetadata{Role: "unknown"}
	found, err := sniffBinary(bytes.NewReader(head), &observed, at)
	if err != nil {
		return err
	}
	mainPrefix := bundle + "/Contents/MacOS/"
	mainEntry := bundle != "" && strings.HasPrefix(name, mainPrefix) && !strings.Contains(strings.TrimPrefix(name, mainPrefix), "/")
	if found && mainEntry {
		if slices.Contains(m.Targets, ArtifactTarget{Platform: "macos"}) && m.Role == "application" {
			m.Role = "unknown"
		}
		m.Targets = slices.DeleteFunc(m.Targets, func(t ArtifactTarget) bool { return t.Platform == "macos" && t.Arch == "" })
		if len(head) >= 4 && (machoOrder(head[:4]) != nil || binary.BigEndian.Uint32(head[:4]) == 0xcafebabe || binary.BigEndian.Uint32(head[:4]) == 0xcafebabf) {
			for i := range observed.Targets {
				if observed.Targets[i].Platform == "" {
					observed.Targets[i].Platform = "macos"
				}
				if observed.Targets[i].Platform != "macos" {
					observed.Conflicts = append(observed.Conflicts, "app bundle conflicts with main executable platform")
				}
			}
		} else {
			observed.Conflicts = append(observed.Conflicts, "app bundle main executable is not Mach-O")
		}
	}
	if (bundle != "" && !mainEntry) || (!mainEntry && observed.Role != "application" && observed.Role != "installer") {
		// Library targets cannot establish an application build for another CPU or OS.
		observed.Targets = nil
	}
	observeRole(m, observed.Role)
	m.Evidence = append(m.Evidence, observed.Evidence...)
	m.Conflicts = append(m.Conflicts, observed.Conflicts...)
	for _, target := range observed.Targets {
		addTarget(m, target)
	}
	return nil
}

// binaryCandidate picks archive entries worth sniffing: Mac bundle
// executables, executable-mode files, .exe/.AppImage, and extensionless
// files that are not common docs. Tiny files are usually scripts.
func binaryCandidate(name string, mode fs.FileMode, size int64) bool {
	if !mode.IsRegular() || size < 512 || strings.HasPrefix(name, "__MACOSX/") {
		return false
	}
	lower := strings.ToLower(name)
	base := path.Base(lower)
	switch {
	case strings.Contains(lower, ".app/contents/macos/"), mode&0o111 != 0,
		hasAnySuffix(lower, ".exe", ".appimage"):
		return true
	case strings.Contains(base, "."), strings.Count(lower, "/") > 1:
		return false
	}
	for _, doc := range []string{"license", "licence", "readme", "copying", "notice", "changelog", "authors"} {
		if strings.HasPrefix(base, doc) {
			return false
		}
	}
	return true
}

func bundleRank(name string) int {
	if _, rest, ok := strings.Cut(name, ".app/Contents/MacOS/"); ok && !strings.Contains(rest, "/") {
		return 0
	}
	return 1
}

// sniffable reports whether a standalone asset name may be a raw executable:
// no extension, .exe/.AppImage/.bin, or a version-like suffix ("tool-1.2.3").
func sniffable(name string) bool {
	ext := strings.TrimPrefix(path.Ext(name), ".")
	if ext == "" || ext == "exe" || ext == "appimage" || ext == "bin" {
		return true
	}
	return strings.ContainsAny(ext, "0123456789-_")
}

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

// note appends inspection evidence; at names the archive entry, if any.
func note(m *ArtifactMetadata, at, field, value string) {
	if at != "" {
		field = at + ":" + field
	}
	m.Evidence = append(m.Evidence, ArtifactEvidence{Source: "inspection", Field: field, Value: value})
}

func addTarget(m *ArtifactMetadata, t ArtifactTarget) {
	for _, have := range m.Targets {
		if have == t {
			return
		}
	}
	m.Targets = append(m.Targets, t)
}

// sniffBinary parses an executable header at offset 0 of r. It returns false
// (and no error) when the magic bytes match no supported format. Optional
// parts beyond the readable bytes (interpreter, notes, fat slices) are
// skipped, so a truncated archive-entry prefix yields less evidence, not an error.
func sniffBinary(r io.ReaderAt, m *ArtifactMetadata, at string) (bool, error) {
	var magic [8]byte
	n, _ := r.ReadAt(magic[:], 0)
	switch {
	case n >= 4 && string(magic[:4]) == "\x7fELF":
		return true, readELF(r, m, at)
	case n >= 2 && string(magic[:2]) == "MZ":
		return readPE(r, m, at)
	case n >= 4 && machoOrder(magic[:4]) != nil:
		return true, readMachO(r, m, at, ArtifactTarget{})
	case n >= 8 && (binary.BigEndian.Uint32(magic[:]) == 0xcafebabe || binary.BigEndian.Uint32(magic[:]) == 0xcafebabf) &&
		binary.BigEndian.Uint32(magic[4:]) >= 1 && binary.BigEndian.Uint32(magic[4:]) <= 16:
		// Java class files share 0xcafebabe; their next word (version) is >= 45.
		return true, readFatMachO(r, m, at, binary.BigEndian.Uint32(magic[:]) == 0xcafebabf)
	}
	return false, nil
}

func malformed(format string) error {
	return fmt.Errorf("%w: truncated or invalid %s header", ErrMalformedArtifact, format)
}

// readELF records machine, OS ABI, PT_INTERP, and ABI-tag notes. The OS is set
// only from OS ABI, interpreter path, or an OS note: ELF magic alone proves
// nothing (static Go binaries for Linux carry none of these).
func readELF(r io.ReaderAt, m *ArtifactMetadata, at string) error {
	h := make([]byte, 64)
	n, _ := r.ReadAt(h, 0)
	if n < 52 || (h[4] == 2 && n < 64) {
		return malformed("ELF")
	}
	var bo binary.ByteOrder
	switch h[5] {
	case 1:
		bo = binary.LittleEndian
	case 2:
		bo = binary.BigEndian
	default:
		return malformed("ELF")
	}
	is64 := h[4] == 2
	if !is64 && h[4] != 1 {
		return malformed("ELF")
	}
	typ, mach := elf.Type(bo.Uint16(h[16:])), elf.Machine(bo.Uint16(h[18:]))
	var phoff int64
	var phentsize, phnum int
	if is64 {
		phoff, phentsize, phnum = int64(bo.Uint64(h[32:])), int(bo.Uint16(h[54:])), int(bo.Uint16(h[56:]))
	} else {
		phoff, phentsize, phnum = int64(bo.Uint32(h[28:])), int(bo.Uint16(h[42:])), int(bo.Uint16(h[44:]))
	}
	note(m, at, "elf.machine", mach.String())
	note(m, at, "elf.type", typ.String())
	oses := map[string]bool{}
	if osabi := elf.OSABI(h[7]); osabi != elf.ELFOSABI_NONE {
		note(m, at, "elf.osabi", osabi.String())
		if os := elfOSABI[osabi]; os != "" {
			oses[os] = true
		} else {
			oses["elf-osabi:"+strconv.Itoa(int(osabi))] = true
		}
	}
	interp, libc := "", ""
	minEntry := 32
	if is64 {
		minEntry = 56
	}
	var ph []byte
	if phentsize >= minEntry && phentsize <= 256 && phnum <= 256 && phoff > 0 {
		ph = make([]byte, phentsize*phnum)
		if _, err := r.ReadAt(ph, phoff); err != nil {
			ph = nil // beyond the readable prefix
		}
	} else {
		ph = nil
	}
	for i := 0; phentsize >= minEntry && i+phentsize <= len(ph); i += phentsize {
		p := ph[i:]
		var off, size int64
		if is64 {
			off, size = int64(bo.Uint64(p[8:])), int64(bo.Uint64(p[32:]))
		} else {
			off, size = int64(bo.Uint32(p[4:])), int64(bo.Uint32(p[16:]))
		}
		if off < 0 || size <= 0 {
			continue
		}
		switch elf.ProgType(bo.Uint32(p)) {
		case elf.PT_INTERP:
			buf := make([]byte, min(size, 256))
			if k, _ := r.ReadAt(buf, off); k == len(buf) {
				interp = string(bytes.TrimRight(buf, "\x00"))
				note(m, at, "elf.interp", interp)
				os, l := interpOS(interp)
				if os != "" {
					oses[os] = true
				}
				libc = l
			}
		case elf.PT_NOTE:
			buf := make([]byte, min(size, 4096))
			if k, _ := r.ReadAt(buf, off); k == len(buf) {
				for _, os := range elfNoteOSes(buf, bo) {
					note(m, at, "elf.note_abi", os)
					oses[os] = true
				}
			}
		}
	}
	switch {
	case typ == elf.ET_EXEC, typ == elf.ET_DYN && interp != "":
		observeRole(m, "application")
	default:
		observeRole(m, "metadata")
	}
	target := ArtifactTarget{Arch: elfArch(mach, is64)}
	switch len(oses) {
	case 0:
	case 1:
		for os := range oses {
			target.Platform = os
		}
	default:
		m.Conflicts = append(m.Conflicts, fmt.Sprintf("%sELF OS evidence disagrees: %v", entryPrefix(at), slices.Sorted(maps.Keys(oses))))
	}
	if target.Platform == "linux" {
		if libc == "" && interp == "" && typ == elf.ET_EXEC {
			libc = "static"
		}
		target.Libc = libc
	}
	addTarget(m, target)
	return nil
}

var elfOSABI = map[elf.OSABI]string{
	elf.ELFOSABI_LINUX:   "linux",
	elf.ELFOSABI_FREEBSD: "freebsd",
	elf.ELFOSABI_NETBSD:  "netbsd",
	elf.ELFOSABI_OPENBSD: "openbsd",
}

// interpOS maps a dynamic loader path to its OS and libc.
func interpOS(interp string) (os, libc string) {
	base := path.Base(interp)
	switch {
	case strings.HasPrefix(base, "ld-musl-"):
		return "linux", "musl"
	case strings.HasPrefix(base, "ld-linux"), strings.HasPrefix(base, "ld64.so."):
		return "linux", "gnu"
	case strings.HasPrefix(interp, "/system/bin/linker"):
		return "android", ""
	case base == "ld-elf.so.1":
		return "freebsd", ""
	case base == "ld.elf_so":
		return "netbsd", ""
	}
	return "", ""
}

// elfNoteOSes reads NT_*_ABI_TAG (type 1) notes: "GNU" with OS word 0 is
// Linux; "FreeBSD", "NetBSD", "OpenBSD", and "Android" name their OS.
func elfNoteOSes(b []byte, bo binary.ByteOrder) []string {
	var oses []string
	for len(b) >= 12 {
		namesz, descsz, typ := int(bo.Uint32(b)), int(bo.Uint32(b[4:])), bo.Uint32(b[8:])
		nameEnd := 12 + (namesz+3)&^3
		descEnd := nameEnd + (descsz+3)&^3
		if namesz < 0 || descsz < 0 || nameEnd > len(b) || descEnd > len(b) || 12+namesz > len(b) {
			break
		}
		name := string(bytes.TrimRight(b[12:12+namesz], "\x00"))
		if typ == 1 {
			switch name {
			case "GNU":
				if descsz >= 4 && bo.Uint32(b[nameEnd:]) == 0 {
					oses = append(oses, "linux")
				}
			case "FreeBSD", "NetBSD", "OpenBSD":
				oses = append(oses, strings.ToLower(name))
			case "Android":
				oses = append(oses, "android")
			}
		}
		b = b[descEnd:]
	}
	return oses
}

func elfArch(m elf.Machine, is64 bool) string {
	switch {
	case m == elf.EM_X86_64 && is64:
		return "amd64"
	case m == elf.EM_AARCH64:
		return "arm64"
	case m == elf.EM_ARM:
		return "arm"
	case m == elf.EM_386:
		return "386"
	case m == elf.EM_RISCV && is64:
		return "riscv64"
	}
	return "elf:" + strconv.FormatUint(uint64(m), 10)
}

// readPE records machine, subsystem, and DLL/CLR flags. A PE image targets
// Windows unless its subsystem is EFI. CLR images keep their machine constraint
// because the CLR header alone does not establish AnyCPU support.
func readPE(r io.ReaderAt, m *ArtifactMetadata, at string) (bool, error) {
	dos := make([]byte, 64)
	if n, _ := r.ReadAt(dos, 0); n < 64 {
		return false, nil // "MZ" text, not a DOS/PE header
	}
	lfanew := int64(binary.LittleEndian.Uint32(dos[0x3c:]))
	h := make([]byte, 24+240)
	n, _ := r.ReadAt(h, lfanew)
	if n < 4 || string(h[:4]) != "PE\x00\x00" {
		return false, nil // plain DOS program or unrelated data
	}
	if n < 24 {
		return true, malformed("PE")
	}
	machine, chars := binary.LittleEndian.Uint16(h[4:]), binary.LittleEndian.Uint16(h[22:])
	optSize := int(binary.LittleEndian.Uint16(h[20:]))
	opt := h[24:min(n, 24+optSize)]
	note(m, at, "pe.machine", fmt.Sprintf("0x%04x", machine))
	target := ArtifactTarget{Platform: "windows", Arch: peArch[machine]}
	if target.Arch == "" {
		target.Arch = fmt.Sprintf("pe:0x%x", machine)
	}
	if len(opt) >= 70 {
		subsystem := binary.LittleEndian.Uint16(opt[68:])
		note(m, at, "pe.subsystem", strconv.Itoa(int(subsystem)))
		if subsystem >= 10 && subsystem <= 13 {
			target.Platform = "uefi"
		}
		dd := 96
		if binary.LittleEndian.Uint16(opt) == 0x20b {
			dd = 112
		}
		if clr := dd + 14*8; len(opt) >= clr+8 && binary.LittleEndian.Uint32(opt[clr:]) != 0 {
			note(m, at, "pe.clr", "present")
			// CLR presence does not prove AnyCPU; 32BITREQUIRED may still constrain x86.
		}
	}
	if chars&0x2000 == 0 && chars&0x0002 != 0 {
		observeRole(m, "application")
	} else {
		observeRole(m, "metadata")
	}
	addTarget(m, target)
	return true, nil
}

var peArch = map[uint16]string{0x8664: "amd64", 0xaa64: "arm64", 0xa641: "arm64", 0x14c: "386", 0x1c4: "arm", 0x5064: "riscv64"}

func machoOrder(b []byte) binary.ByteOrder {
	switch binary.LittleEndian.Uint32(b) {
	case 0xfeedface, 0xfeedfacf:
		return binary.LittleEndian
	}
	switch binary.BigEndian.Uint32(b) {
	case 0xfeedface, 0xfeedfacf:
		return binary.BigEndian
	}
	return nil
}

// readMachO records CPU, file type, and the platform from LC_BUILD_VERSION or
// LC_VERSION_MIN_*. Without either command the Apple OS stays unknown. fallback
// carries the fat-header CPU for slices inside a universal binary.
func readMachO(r io.ReaderAt, m *ArtifactMetadata, at string, fallback ArtifactTarget) error {
	h := make([]byte, 32)
	n, _ := r.ReadAt(h, 0)
	bo := machoOrder(h)
	if n < 28 || bo == nil {
		return malformed("Mach-O")
	}
	is64 := bo.Uint32(h)&1 == 1
	if is64 && n < 32 {
		return malformed("Mach-O")
	}
	cpu, filetype := bo.Uint32(h[4:]), bo.Uint32(h[12:])
	ncmds, sizeofcmds := int(bo.Uint32(h[16:])), int64(bo.Uint32(h[20:]))
	note(m, at, "macho.cputype", fmt.Sprintf("0x%x", cpu))
	note(m, at, "macho.filetype", strconv.Itoa(int(filetype)))
	switch {
	case filetype == 2: // MH_EXECUTE
		observeRole(m, "application")
	case filetype == 0xa: // MH_DSYM
		observeRole(m, "debug")
	default:
		observeRole(m, "metadata")
	}
	target := ArtifactTarget{Arch: machoArch[cpu]}
	if target.Arch == "" {
		target.Arch = fallback.Arch
		if target.Arch == "" {
			target.Arch = fmt.Sprintf("macho:0x%x", cpu)
		}
	}
	hdr := int64(28)
	if is64 {
		hdr = 32
	}
	cmds := make([]byte, min(sizeofcmds, 1<<20))
	if k, _ := r.ReadAt(cmds, hdr); k < len(cmds) {
		cmds = cmds[:k] // parse what the prefix holds
	}
	for i := 0; i < ncmds && len(cmds) >= 8; i++ {
		cmd, size := bo.Uint32(cmds), int(bo.Uint32(cmds[4:]))
		if size < 8 || size > len(cmds) {
			break
		}
		platform := ""
		switch cmd {
		case 0x32: // LC_BUILD_VERSION
			if size >= 12 {
				p := bo.Uint32(cmds[8:])
				note(m, at, "macho.build_platform", strconv.Itoa(int(p)))
				platform = machoPlatform[p]
				if platform == "" {
					platform = "macho-platform:" + strconv.FormatUint(uint64(p), 10)
				}
			}
		case 0x24: // LC_VERSION_MIN_MACOSX
			platform = "macos"
		case 0x25: // LC_VERSION_MIN_IPHONEOS
			platform = "ios"
		case 0x2f: // LC_VERSION_MIN_TVOS
			platform = "tvos"
		case 0x30: // LC_VERSION_MIN_WATCHOS
			platform = "watchos"
		}
		if platform != "" && target.Platform != "" && platform != target.Platform {
			m.Conflicts = append(m.Conflicts, fmt.Sprintf("%sMach-O platforms disagree: %s, %s", entryPrefix(at), target.Platform, platform))
			target.Platform = ""
			break
		}
		if platform != "" {
			target.Platform = platform
		}
		cmds = cmds[size:]
	}
	addTarget(m, target)
	return nil
}

var machoArch = map[uint32]string{0x01000007: "amd64", 0x0100000c: "arm64", 7: "386", 12: "arm"}

// machoPlatform maps LC_BUILD_VERSION platforms (<mach-o/loader.h>).
// Mac Catalyst runs on macOS; simulators are not end-user targets.
var machoPlatform = map[uint32]string{
	1: "macos", 2: "ios", 3: "tvos", 4: "watchos", 5: "bridgeos", 6: "macos",
	7: "ios-simulator", 8: "tvos-simulator", 9: "watchos-simulator", 10: "driverkit",
	11: "visionos", 12: "visionos-simulator",
}

// readFatMachO reads the universal header, then each slice header that is
// readable (at most 8). Unreadable slices keep their CPU with an unknown OS.
func readFatMachO(r io.ReaderAt, m *ArtifactMetadata, at string, fat64 bool) error {
	entry := 20
	if fat64 {
		entry = 32
	}
	head := make([]byte, 8)
	r.ReadAt(head, 0)
	count := min(int(binary.BigEndian.Uint32(head[4:])), 8)
	arches := make([]byte, entry*count)
	if n, _ := r.ReadAt(arches, 8); n < len(arches) {
		return malformed("fat Mach-O")
	}
	note(m, at, "macho.fat_arches", strconv.Itoa(count))
	for i := range count {
		a := arches[i*entry:]
		cpu := binary.BigEndian.Uint32(a)
		var off, size int64
		if fat64 {
			off, size = int64(binary.BigEndian.Uint64(a[8:])), int64(binary.BigEndian.Uint64(a[16:]))
		} else {
			off, size = int64(binary.BigEndian.Uint32(a[8:])), int64(binary.BigEndian.Uint32(a[12:]))
		}
		fallback := ArtifactTarget{Arch: machoArch[cpu]}
		if fallback.Arch == "" {
			fallback.Arch = fmt.Sprintf("macho:0x%x", cpu)
		}
		slice := io.NewSectionReader(r, off, size)
		probe := make([]byte, 4)
		if k, _ := slice.ReadAt(probe, 0); k < 4 {
			note(m, at, "macho.fat_slice", fmt.Sprintf("0x%x unread", cpu))
			addTarget(m, fallback)
			continue
		}
		if err := readMachO(slice, m, at, fallback); err != nil {
			return err
		}
	}
	return nil
}

func entryPrefix(at string) string {
	if at == "" {
		return ""
	}
	return at + ": "
}
