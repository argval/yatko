package github

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ulikunitz/xz"
)

func TestDeepDownloadBoundsRedirectsAndIdentity(t *testing.T) {
	data := []byte("public release bytes")
	requests := 0
	redirect := "https://release-assets.githubusercontent.com/blob?signature=private"
	c := &Client{token: "must-not-leak", httpClient: &http.Client{Transport: archiveTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Range") != "" || r.Header.Get("Accept-Encoding") != "identity" {
			t.Fatal("download carried credentials, a range, or transparent compression")
		}
		if r.URL.Host == "github.com" {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {redirect}}, Body: http.NoBody}, nil
		}
		return &http.Response{StatusCode: 200, ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}}
	d := &DeepInspector{DownloadDir: t.TempDir(), client: c}
	a := Asset{ID: 1, Name: "../not-a-path.exe", Size: int64(len(data)), BrowserDownloadURL: "https://github.com/a/b/releases/download/v1/app.exe"}
	first, cleanup, err := d.download(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if filepath.Dir(first) != d.DownloadDir || strings.Contains(filepath.Base(first), "not-a-path") {
		t.Fatal("asset filename escaped the generated cache name")
	}
	second, _, err := d.download(context.Background(), a)
	if err != nil || first != second || requests != 2 {
		t.Fatalf("complete file was not reused: %s %v requests=%d", second, err, requests)
	}
	a.UpdatedAt = "replacement"
	third, _, err := d.download(context.Background(), a)
	if err != nil || first == third || requests != 4 {
		t.Fatal("same-URL asset replacement reused stale bytes")
	}
	redirect = "https://attacker.invalid/?signature=private"
	a.UpdatedAt = "another replacement"
	_, _, err = d.download(context.Background(), a)
	if err == nil || strings.Contains(err.Error(), "signature") {
		t.Fatalf("refused redirect exposed its signed URL: %v", err)
	}
}

func TestDeepDownloadRejectsPartialAndOversizedBodies(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{http.StatusPartialContent, "1234"}, {http.StatusOK, "123"}, {http.StatusOK, "12345"},
	} {
		d := &DeepInspector{DownloadDir: t.TempDir(), client: &Client{httpClient: &http.Client{Transport: archiveTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, ContentLength: -1, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}}}
		a := Asset{Name: "tool.zip", Size: 4, BrowserDownloadURL: "https://github.com/a/b/releases/download/v1/tool.zip"}
		if _, _, err := d.download(context.Background(), a); err == nil {
			t.Fatalf("accepted HTTP %d body %q", tc.status, tc.body)
		}
		files, err := os.ReadDir(d.DownloadDir)
		if err != nil || len(files) != 0 {
			t.Fatalf("incomplete download was retained: %v %v", files, err)
		}
	}
}

func TestDeepLocalFileRequiresExactBytesBeforeExtraction(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "artifact")
	data := []byte("not an installer")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	d := &DeepInspector{SevenZip: "/tool-must-not-run"}
	a := Asset{Name: "tool.zip", Size: int64(len(data)), Digest: "sha256:wrong", BrowserDownloadURL: "https://github.com/a/b/releases/download/v1/tool.zip"}
	if _, err := d.InspectFile(context.Background(), a, filename); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatal("unverified file reached extraction", err)
	}
	a.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	a.Size++
	if _, err := d.InspectFile(context.Background(), a, filename); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatal("size mismatch reached extraction", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Size--
	if _, err := d.InspectFile(ctx, a, filename); !errors.Is(err, context.Canceled) {
		t.Fatal("hashing ignored canceled work", err)
	}
}

func TestDeepListingHandlesUnknownNSISSizesAndSkipsLinks(t *testing.T) {
	listing := "Type = Nsis\n----------\nPath = App.exe\nSize = \n\nPath = link\nSize = 400\nSymbolic Link = ../../outside\n\nPath = folder\nType = directory\n\n"
	kind, entries, err := parseDeepListing([]byte(listing))
	if err != nil || kind != "Nsis" || len(entries) != 2 || entries[0].size != -1 || !entries[1].directory {
		t.Fatalf("listing=%+v %q %v", entries, kind, err)
	}
	_, _, err = parseDeepListing([]byte("Type = zip\n----------\nPath = App.exe\nSize = 40\nSize = 80\n"))
	if err == nil {
		t.Fatal("ambiguous record was accepted")
	}
}

func TestDeepExecutableSelectionIgnoresScriptsAndSystemHelpers(t *testing.T) {
	a := deepAppFixture(map[string][]byte{
		"LICENSE":                  bytes.Repeat([]byte("text"), 20),
		"bin/launch":               append([]byte("#!/bin/sh\n"), bytes.Repeat([]byte("#"), 90)...),
		"bin/main":                 elfFixture(elf.ELFOSABI_LINUX, elf.ET_EXEC, "", false),
		"$PLUGINSDIR/helper.exe":   fixture(t, "go-windows-amd64.head"),
		"lib/app/crash_report.exe": fixture(t, "go-windows-amd64.head"),
	})
	m, found, err := a.inspectExecutables("deliberately-wrong-arm64.zip")
	if err != nil || !found || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "static"}}) {
		t.Fatalf("metadata=%+v found=%v error=%v", m, found, err)
	}
}

func TestDeepRuntimeLibrariesAndLibraryOnlyArchive(t *testing.T) {
	libs := map[string][]byte{
		"tool/bin/run":                      []byte("#!/bin/sh\nexec java -cp tool.jar Main\n"),
		"tool/konan/lib/tool.jar":           []byte("jar"),
		"tool/konan/nativelib/native.dylib": machoFixture(0x0100000c, 6, 1),
	}
	m, found, err := deepAppFixture(libs).inspectRuntimeLibraries()
	if err != nil || !found || !deepProven(m) || m.Role != "application" || m.Targets[0].Arch != "arm64" {
		t.Fatalf("JVM metadata=%+v found=%v error=%v", m, found, err)
	}
	m, found, err = deepAppFixture(map[string][]byte{"libs/only.prx": elfFixture(0, elf.ET_DYN, "", false)}).inspectRuntimeLibraries()
	if err != nil || !found || m.Status != "inspected" || m.Role != "metadata" || len(m.Targets) != 0 {
		t.Fatalf("library metadata=%+v found=%v error=%v", m, found, err)
	}
	libs["tool/konan/nativelib/other.dylib"] = machoFixture(0x01000007, 6, 1)
	m, _, err = deepAppFixture(libs).inspectRuntimeLibraries()
	if err != nil || deepProven(m) {
		t.Fatal("conflicting runtime CPU targets became a universal app", m, err)
	}
}

func TestDeepEntryReaderRejectsShortReadWithoutLoop(t *testing.T) {
	a := &deepArchive{readEntry: func(deepEntry, int64, int64) ([]byte, error) { return nil, nil }}
	r := &deepEntryReader{archive: a, entry: deepEntry{path: "bad", size: 1000}}
	if _, err := r.ReadAt(make([]byte, 64), 0); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("short extractor read did not fail", err)
	}
}

func TestDeepExtractorOutputAndCancellationAreBounded(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "trusted-fixture")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nwhile :; do printf '1234567890'; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s := &deepSession{ctx: ctx, dir: dir}
	data, err := s.run(script, nil, 100)
	if len(data) != 100 || !errors.Is(err, errInspectLimit) {
		t.Fatalf("output=%d error=%v", len(data), err)
	}
}

func TestBurnContainerFraming(t *testing.T) {
	le := binary.LittleEndian
	b := make([]byte, 2048)
	copy(b, "MZ")
	le.PutUint32(b[60:], 64)
	copy(b[64:], "PE\x00\x00")
	le.PutUint16(b[70:], 1)
	le.PutUint16(b[84:], 240)
	section := b[64+24+240:]
	copy(section, ".wixburn")
	le.PutUint32(section[16:], 512)
	le.PutUint32(section[20:], 512)
	header := b[512:]
	le.PutUint32(header, 0x00f14300)
	le.PutUint32(header[4:], 2)
	le.PutUint32(header[24:], 1024)
	le.PutUint32(header[40:], 1)
	le.PutUint32(header[44:], 2)
	le.PutUint32(header[48:], 128)
	le.PutUint32(header[52:], 256)
	copy(b[1024:], "MSCF")
	copy(b[1152:], "MSCF")
	containers, found, err := readBurnContainers(bytes.NewReader(b), int64(len(b)))
	if err != nil || !found || !slices.Equal(containers, []burnContainer{{1024, 128}, {1152, 256}}) {
		t.Fatalf("containers=%v found=%v error=%v", containers, found, err)
	}
	le.PutUint32(header[52:], 0xffffffff)
	_, found, err = readBurnContainers(bytes.NewReader(b), int64(len(b)))
	if !found || !errors.Is(err, ErrMalformedArtifact) {
		t.Fatal("out-of-file attached container was accepted", err)
	}
}

// An Inno installer may ship one copy of each program per CPU. Only CPU checks
// that match each copy, without overlap per program, may widen the targets.
func TestCPUGatedInnoTargets(t *testing.T) {
	win := func(arch string) ArtifactMetadata {
		return ArtifactMetadata{Status: "inspected", Role: "application", Targets: []ArtifactTarget{{Platform: "windows", Arch: arch}}}
	}
	// Ollama: two programs, each shipped as x64 and arm64.
	four := []ArtifactMetadata{win("amd64"), win("amd64"), win("arm64"), win("arm64")}
	dests := []string{"{app}\\ollama app.exe", "{app}\\ollama.exe", "{app}\\ollama app.exe", "{app}\\ollama.exe"}
	m, ok := cpuGatedTargets(four, []string{"not IsArm64()", "not IsArm64()", "IsArm64()", "IsArm64()"}, dests)
	if !ok || len(m.Targets) != 2 {
		t.Fatalf("gated dual-CPU installer: %+v %v", m, ok)
	}
	both := []ArtifactMetadata{win("amd64"), win("arm64")}
	one := []string{"{app}\\app.exe", "{app}\\app.exe"}
	for name, gates := range map[string][]string{
		"ungated":           {"", ""},
		"gate mismatches":   {"IsArm64()", "not IsArm64()"},
		"overlapping gates": {"not IsArm64()", "IsX64Compatible()"},
		"one copy ungated":  {"not IsArm64()", ""},
	} {
		if _, ok := cpuGatedTargets(both, gates, one); ok {
			t.Errorf("%s: widened targets", name)
		}
	}
	// A second program that exists only for x64 must not inherit arm64.
	three := []ArtifactMetadata{win("amd64"), win("arm64"), win("amd64")}
	if _, ok := cpuGatedTargets(three, []string{"not IsArm64()", "IsArm64()", "not IsArm64()"},
		[]string{"{app}\\a.exe", "{app}\\a.exe", "{app}\\b.exe"}); ok {
		t.Error("programs covering different CPUs widened targets")
	}
}

// A pbzx stream (macOS flat-package Payload) decodes chunk by chunk, both xz
// and stored chunks; truncated or oversized chunks are rejected.
func TestDecodePBZX(t *testing.T) {
	xzChunk := func(data []byte) []byte {
		var buf bytes.Buffer
		w, err := xz.NewWriter(&buf)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
		w.Close()
		return buf.Bytes()
	}
	be := func(v uint64) []byte { return binary.BigEndian.AppendUint64(nil, v) }
	stream := func(chunks ...[]byte) []byte {
		b := append([]byte("pbzx"), be(1<<24)...)
		for i, c := range chunks {
			flags := uint64(1 << 24)
			if i == len(chunks)-1 {
				flags = 0
			}
			b = append(append(append(b, be(flags)...), be(uint64(len(c)))...), c...)
		}
		return b
	}
	dir := t.TempDir()
	s := &deepSession{ctx: context.Background(), dir: dir}
	write := func(b []byte) string {
		f := filepath.Join(dir, fmt.Sprintf("in-%d", len(b)))
		if err := os.WriteFile(f, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return f
	}
	in := write(stream(xzChunk([]byte("070707first ")), []byte("stored")))
	if !isPBZX(in) {
		t.Fatal("pbzx magic not recognized")
	}
	out, err := s.decodePBZX(in)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "070707first stored" {
		t.Fatalf("decoded %q", got)
	}
	bad := stream([]byte("x"))
	binary.BigEndian.PutUint64(bad[20:], pbzxMaxChunk+1)
	if _, err := s.decodePBZX(write(bad)); !errors.Is(err, ErrMalformedArtifact) {
		t.Fatalf("oversized chunk: %v", err)
	}
	if _, err := s.decodePBZX(write(stream([]byte("abc"))[:24])); !errors.Is(err, ErrMalformedArtifact) {
		t.Fatalf("truncated chunk: %v", err)
	}
}
