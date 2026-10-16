package github

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"debug/elf"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
)

// runInspector runs one inspector over a fake release download, as
// InspectArtifact would after dispatch.
func runInspector(t *testing.T, inspect func(*remoteFile, *ArtifactMetadata) error, data []byte) (ArtifactMetadata, error, *remoteFile) {
	t.Helper()
	s := newFakeDownloads(t)
	asset := s.add("asset", data)
	f := s.client().openAsset(context.Background(), asset.BrowserDownloadURL, asset.Size)
	m := ArtifactMetadata{Role: "unknown"}
	err := inspect(f, &m)
	if s.requests > inspectMaxRequests {
		t.Fatalf("requests = %d", s.requests)
	}
	return m, err, f
}

func hasEvidence(m ArtifactMetadata, prefix string) bool {
	return slices.ContainsFunc(m.Evidence, func(e ArtifactEvidence) bool { return strings.HasPrefix(e.Field, prefix) })
}

func plainTar(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(tarGzFixture(t, entries...)))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func xzFixture(t *testing.T, data []byte, blockSize int64) []byte {
	t.Helper()
	var b bytes.Buffer
	w, err := xz.WriterConfig{DictCap: 1 << 16, BlockSize: blockSize}.NewWriter(&b)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(data)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// patchXZFilter rewrites the first block's LZMA2 filter ID and property byte
// and fixes the header CRC, so only the patched field is wrong.
func patchXZFilter(t *testing.T, data []byte, id, prop byte) []byte {
	t.Helper()
	data = slices.Clone(data)
	h := data[12 : 12+(int(data[12])+1)*4]
	i := bytes.Index(h, []byte{0x21, 0x01})
	if i < 0 {
		t.Fatal("no LZMA2 filter in block header")
	}
	h[i], h[i+2] = id, prop
	binary.LittleEndian.PutUint32(h[len(h)-4:], crc32.ChecksumIEEE(h[:len(h)-4]))
	return data
}

func zstdFixture(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w, err := zstd.NewWriter(&b)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(data)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// szNumber encodes 7z's variable-length UINT64.
func szNumber(v uint64) []byte {
	n := 0
	for n < 8 && v >= 1<<(7*(n+1)) {
		n++
	}
	b := []byte{byte(0xff<<(8-n)) | byte(v>>(8*n))}
	if n == 8 {
		b[0] = 0xff
	}
	for i := range n {
		b = append(b, byte(v>>(8*i)))
	}
	return b
}

type szOptions struct {
	lzma2        bool   // compress the folder (otherwise Copy)
	encodeHeader bool   // LZMA2-encode the header, as 7-Zip does by default
	coder        string // overrides the content coder ID, e.g. AES
	headerCoder  string // overrides the encoded header's coder ID
}

// sevenZipFixture packs entries into one solid folder with Unix modes in the
// attribute high bits, following 7zFormat.txt.
func sevenZipFixture(t *testing.T, o szOptions, entries ...tarEntry) []byte {
	t.Helper()
	lzma2Pack := func(data []byte) ([]byte, []byte) {
		var b bytes.Buffer
		w, err := lzma.Writer2Config{DictCap: 1 << 16}.NewWriter2(&b)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		return b.Bytes(), []byte{lzma.EncodeDictCap(1 << 16)}
	}
	var content []byte
	for _, e := range entries {
		content = append(content, e.data...)
	}
	id, props, packed := "\x00", []byte(nil), content
	if o.lzma2 {
		packed, props = lzma2Pack(content)
		id = "\x21"
	}
	if o.coder != "" {
		id = o.coder
	}
	streams := func(packPos, packSize, unpack uint64, id string, props []byte, crc *uint32) []byte {
		b := []byte{0x06}
		b = append(append(append(b, szNumber(packPos)...), 1, 0x09), szNumber(packSize)...)
		b = append(b, 0, 0x07, 0x0b, 1, 0)
		flags := byte(len(id))
		if props != nil {
			flags |= 0x20
		}
		b = append(append(b, 1, flags), id...)
		if props != nil {
			b = append(append(b, szNumber(uint64(len(props)))...), props...)
		}
		b = append(append(b, 0x0c), szNumber(unpack)...)
		if crc != nil {
			b = binary.LittleEndian.AppendUint32(append(b, 0x0a, 1), *crc)
		}
		return append(b, 0, 0) // end UnpackInfo, end StreamsInfo
	}
	h := append([]byte{0x01, 0x04}, streams(0, uint64(len(packed)), uint64(len(content)), id, props, nil)...)
	h = h[:len(h)-1] // reopen StreamsInfo for SubStreamsInfo
	h = append(append(h, 0x08, 0x0d), szNumber(uint64(len(entries)))...)
	h = append(h, 0x09)
	for _, e := range entries[:len(entries)-1] {
		h = append(h, szNumber(uint64(len(e.data)))...)
	}
	h = append(h, 0, 0)
	var names, attrs []byte
	names = append(names, 0)
	attrs = append(attrs, 1, 0)
	for _, e := range entries {
		for _, u := range utf16.Encode([]rune(e.name)) {
			names = binary.LittleEndian.AppendUint16(names, u)
		}
		names = append(names, 0, 0)
		attrs = binary.LittleEndian.AppendUint32(attrs, 0x8000|uint32(0o100000|e.mode)<<16)
	}
	h = append(append(h, 0x05), szNumber(uint64(len(entries)))...)
	h = append(append(append(h, 0x11), szNumber(uint64(len(names)))...), names...)
	h = append(append(append(h, 0x15), szNumber(uint64(len(attrs)))...), attrs...)
	h = append(h, 0, 0)
	body := packed
	if o.encodeHeader {
		hp, hprops := lzma2Pack(h)
		crc := crc32.ChecksumIEEE(h)
		body = append(slices.Clone(packed), hp...)
		hid := cmp.Or(o.headerCoder, "\x21")
		h = append([]byte{0x17}, streams(uint64(len(packed)), uint64(len(hp)), uint64(len(h)), hid, hprops, &crc)...)
	}
	sig := []byte("7z\xbc\xaf\x27\x1c\x00\x04")
	sig = append(sig, make([]byte, 24)...)
	binary.LittleEndian.PutUint64(sig[12:], uint64(len(body)))
	binary.LittleEndian.PutUint64(sig[20:], uint64(len(h)))
	binary.LittleEndian.PutUint32(sig[28:], crc32.ChecksumIEEE(h))
	binary.LittleEndian.PutUint32(sig[8:], crc32.ChecksumIEEE(sig[12:32]))
	return append(append(sig, body...), h...)
}

func TestBoundedArchiveInspectors(t *testing.T) {
	musl := elfFixture(0, elf.ET_DYN, "/lib/ld-musl-x86_64.so.1", false)
	linux := []ArtifactTarget{{Platform: "linux", Arch: "amd64", Libc: "musl"}}
	tarball := plainTar(t, tarEntry{"tool/README.md", 0o644, bytes.Repeat([]byte("docs "), 400)}, tarEntry{"tool/tool", 0o755, musl})
	mac := []tarEntry{{"App.app/Contents/Info.plist", 0o644, []byte("<plist/>")}, {"App.app/Contents/MacOS/App", 0o755, fixture(t, "go-darwin-arm64.head")}}
	for _, tt := range []struct {
		name     string
		inspect  func(*remoteFile, *ArtifactMetadata) error
		data     []byte
		targets  []ArtifactTarget
		evidence string
	}{
		{"xz tar, multi-block", inspectXZTar, xzFixture(t, tarball, 1024), linux, "tar:tool/tool:elf.interp"},
		{"zstd tar", inspectZstdTar, zstdFixture(t, tarball), linux, "tar:tool/tool:elf.interp"},
		{"xz binary", inspectXZ, xzFixture(t, musl, 0), linux, "xz:elf.interp"},
		{"zstd binary", inspectZstd, zstdFixture(t, musl), linux, "zstd:elf.interp"},
		{"7z copy, plain header", inspect7z, sevenZipFixture(t, szOptions{}, mac...), []ArtifactTarget{{Platform: "macos", Arch: "arm64"}}, "7z.app_bundle"},
		{"7z solid lzma2, encoded header", inspect7z, sevenZipFixture(t, szOptions{lzma2: true, encodeHeader: true},
			tarEntry{"tool/README.md", 0o644, bytes.Repeat([]byte("docs "), 400)}, tarEntry{"tool/tool", 0o755, musl}), linux, "7z:tool/tool:elf.interp"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, err, _ := runInspector(t, tt.inspect, tt.data)
			if err != nil || !slices.Equal(m.Targets, tt.targets) || m.Role != "application" {
				t.Fatalf("err=%v targets=%v role=%s evidence=%v", err, m.Targets, m.Role, m.Evidence)
			}
			if !hasEvidence(m, tt.evidence) {
				t.Fatalf("missing %s evidence in %v", tt.evidence, m.Evidence)
			}
		})
	}
}

// A declared 4 GiB xz dictionary is clamped, not allocated: the decode still
// works because the stream never reaches back further than its own output.
func TestXZHugeDictionaryIsClamped(t *testing.T) {
	musl := elfFixture(0, elf.ET_DYN, "/lib/ld-musl-x86_64.so.1", false)
	data := patchXZFilter(t, xzFixture(t, musl, 0), 0x21, 40)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	m, err, _ := runInspector(t, inspectXZ, data)
	runtime.ReadMemStats(&after)
	if err != nil || len(m.Targets) != 1 {
		t.Fatalf("err=%v targets=%v evidence=%v", err, m.Targets, m.Evidence)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 2*decoderDictMax {
		t.Fatalf("allocated %d bytes", alloc)
	}
}

func TestBoundedDecodersRefuseOrFail(t *testing.T) {
	musl := elfFixture(0, elf.ET_DYN, "/lib/ld-musl-x86_64.so.1", false)
	xzData := xzFixture(t, musl, 0)
	z := zstdFixture(t, musl)
	// Frame header declaring a 32 MiB window (descriptor 0x78), then an empty last raw block.
	bigWindow := []byte{0x28, 0xb5, 0x2f, 0xfd, 0x00, 0x78, 0x01, 0x00, 0x00}
	entries := []tarEntry{{"tool", 0o755, musl}}
	plain := sevenZipFixture(t, szOptions{lzma2: true}, entries...)
	badCRC := slices.Clone(plain)
	badCRC[len(badCRC)-3] ^= 0xff
	hugeCount := sevenZipFixture(t, szOptions{}, entries...)
	// Replace the header with "Header, FilesInfo, NumFiles = 2^40".
	h := append([]byte{0x01, 0x05}, szNumber(1<<40)...)
	hugeCount = append(hugeCount[:32+binary.LittleEndian.Uint64(hugeCount[12:])], h...)
	binary.LittleEndian.PutUint64(hugeCount[20:], uint64(len(h)))
	binary.LittleEndian.PutUint32(hugeCount[28:], crc32.ChecksumIEEE(h))
	binary.LittleEndian.PutUint32(hugeCount[8:], crc32.ChecksumIEEE(hugeCount[12:32]))
	bomb := plainTar(t, tarEntry{"zeros", 0o644, make([]byte, 16<<20)}, tarEntry{"tool", 0o755, musl})

	for _, tt := range []struct {
		name     string
		inspect  func(*remoteFile, *ArtifactMetadata) error
		data     []byte
		failed   bool   // a malformed-content error is expected
		evidence string // otherwise: no targets and this note
	}{
		{"xz BCJ-like filter", inspectXZ, patchXZFilter(t, xzData, 0x04, 0), false, "xz.decoder"},
		{"zstd window over cap", inspectZstd, bigWindow, false, "zstd.decoder"},
		{"xz tar inflate cap", inspectXZTar, xzFixture(t, bomb, 0), false, "tar.listing"},
		{"zstd tar inflate cap", inspectZstdTar, zstdFixture(t, bomb), false, "tar.listing"},
		{"7z encrypted folder keeps listing", inspect7z, sevenZipFixture(t, szOptions{coder: "\x06\xf1\x07\x01"}, entries...), false, "7z:tool:decoder"},
		{"7z encrypted header", inspect7z, sevenZipFixture(t, szOptions{encodeHeader: true, headerCoder: "\x06\xf1\x07\x01"}, entries...), false, "7z.header"},
		{"truncated xz", inspectXZ, xzData[:len(xzData)/2], true, ""},
		{"truncated zstd", inspectZstd, z[:len(z)/2], true, ""},
		{"truncated 7z", inspect7z, plain[:len(plain)-10], true, ""},
		{"7z header CRC", inspect7z, badCRC, true, ""},
		{"7z untrusted file count", inspect7z, hugeCount, true, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, err, _ := runInspector(t, tt.inspect, tt.data)
			if tt.failed {
				if err == nil || errors.Is(err, errInspectLimit) {
					t.Fatalf("err = %v, want decode failure (evidence %v)", err, m.Evidence)
				}
				return
			}
			if err != nil || len(m.Targets) != 0 || !hasEvidence(m, tt.evidence) {
				t.Fatalf("err=%v targets=%v evidence=%v", err, m.Targets, m.Evidence)
			}
		})
	}
}

func TestBoundedInspectorsReportTransportErrors(t *testing.T) {
	data := sevenZipFixture(t, szOptions{}, tarEntry{"tool", 0o755, make([]byte, 600)})
	s := newFakeDownloads(t)
	asset := s.add("a.7z", data)
	s.status = 500
	f := s.client().openAsset(context.Background(), asset.BrowserDownloadURL, asset.Size)
	var m ArtifactMetadata
	if err := inspect7z(f, &m); err == nil || f.err == nil || len(m.Evidence) != 0 {
		t.Fatalf("err=%v f.err=%v evidence=%v", err, f.err, m.Evidence)
	}
}
