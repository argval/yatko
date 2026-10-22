package github

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/ulikunitz/xz/lzma"
)

// nsisFixture wraps real PE headers in NSIS file records and compressed blocks.
func nsisFixture(t *testing.T, names []string, solid bool) []byte {
	t.Helper()
	le := binary.LittleEndian
	stringsBlock := []byte{0, 0}
	entries := make([]byte, len(names)*28)
	var data []byte
	for i, name := range names {
		e := entries[i*28:]
		le.PutUint32(e, 20)
		le.PutUint32(e[8:], uint32(len(stringsBlock)/2))
		le.PutUint32(e[12:], uint32(len(data)))
		for _, u := range utf16.Encode([]rune(name + "\x00")) {
			stringsBlock = le.AppendUint16(stringsBlock, u)
		}
		payload := fixture(t, "go-windows-amd64.head")
		pe := le.Uint32(payload[0x3c:])
		le.PutUint16(payload[pe+4:], 0xaa64)
		if strings.HasSuffix(name, ".dll") {
			le.PutUint16(payload[pe+4:], 0x14c)
			le.PutUint16(payload[pe+22:], 0x2002)
		}
		data = le.AppendUint32(data, uint32(len(payload)))
		data = append(data, payload...)
	}
	header := make([]byte, 300)
	le.PutUint32(header[20:], uint32(len(header)))
	le.PutUint32(header[24:], uint32(len(names)))
	le.PutUint32(header[28:], uint32(len(header)+len(entries)))
	le.PutUint32(header[36:], uint32(len(header)+len(entries)+len(stringsBlock)))
	header = append(append(header, entries...), stringsBlock...)
	var block bytes.Buffer
	if solid {
		w, err := (lzma.WriterConfig{DictCap: 1 << 20}).NewWriter(&block)
		if err != nil {
			t.Fatal(err)
		}
		decoded := le.AppendUint32(nil, uint32(len(header)))
		decoded = append(append(decoded, header...), data...)
		if _, err := w.Write(decoded); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		b := block.Bytes()
		block.Reset()
		// NSIS omits the classic LZMA stream's 8-byte output-size field.
		block.Write(append(append([]byte{}, b[:5]...), b[13:]...))
	} else {
		var compressed bytes.Buffer
		w, err := flate.NewWriter(&compressed, flate.BestSpeed)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(header)
		w.Close()
		block.Write(le.AppendUint32(nil, uint32(compressed.Len())|0x80000000))
		block.Write(compressed.Bytes())
		block.Write(data)
	}
	stub := fixture(t, "go-windows-amd64.head")
	le.PutUint16(stub[le.Uint32(stub[0x3c:])+4:], 0x14c)
	stub = append(stub, make([]byte, 512-len(stub)%512)...)
	h := make([]byte, 28)
	copy(h[4:], "\xef\xbe\xad\xdeNullsoftInst")
	le.PutUint32(h[20:], uint32(len(header)))
	le.PutUint32(h[24:], uint32(28+block.Len()))
	return append(append(stub, h...), block.Bytes()...)
}

func TestNSISPayloadInspection(t *testing.T) {
	for _, solid := range []bool{false, true} {
		for _, tc := range []struct {
			name  string
			files []string
			want  []ArtifactTarget
		}{
			{"app archive", []string{"\x03肚\\app-64.7z"}, []ArtifactTarget{{Platform: "windows", Arch: "amd64"}}},
			{"multi CPU installer", []string{"app-64.7z", "app-arm64.7z"}, []ArtifactTarget{{Platform: "windows", Arch: "amd64"}, {Platform: "windows", Arch: "arm64"}}},
			{"app PE excludes x86 plugin", []string{"plugin.dll", "main.exe"}, []ArtifactTarget{{Platform: "windows", Arch: "arm64"}}},
			{"ambiguous apps", []string{"first.exe", "second.exe"}, []ArtifactTarget{{Platform: "windows"}}},
			{"helper beside variable app path", []string{"helper.exe", "\x03肚\\main.exe"}, []ArtifactTarget{{Platform: "windows"}}},
			{"no app CPU", []string{"plugin.dll"}, []ArtifactTarget{{Platform: "windows"}}},
			{"incidental marker", []string{"readme-app-64.7z.txt"}, []ArtifactTarget{{Platform: "windows"}}},
		} {
			t.Run(fmt.Sprintf("%s/solid=%v", tc.name, solid), func(t *testing.T) {
				s := newFakeDownloads(t)
				a := s.add("setup.exe", nsisFixture(t, tc.files, solid))
				m, err := s.client().InspectArtifact(context.Background(), a)
				if err != nil || m.Status != "inspected" || m.Role != "installer" || !slices.Equal(m.Targets, tc.want) {
					t.Fatalf("metadata=%+v err=%v", m, err)
				}
				if s.requests > inspectMaxRequests {
					t.Fatalf("requests=%d", s.requests)
				}
			})
		}
	}
}

func TestNSISBounds(t *testing.T) {
	b := nsisFixture(t, []string{"main.exe"}, true)
	off := bytes.Index(b, []byte("\xef\xbe\xad\xdeNullsoftInst")) - 4
	for _, tc := range []struct {
		name string
		edit func([]byte)
		fail bool
	}{
		{"oversized header", func(b []byte) { binary.LittleEndian.PutUint32(b[off+20:], inspectMaxInflated+1) }, false},
		{"invalid total", func(b []byte) { binary.LittleEndian.PutUint32(b[off+24:], 0xffffffff) }, true},
		{"oversized dictionary", func(b []byte) { binary.LittleEndian.PutUint32(b[off+29:], decoderDictMax+1) }, false},
		{"unsupported solid codec", func(b []byte) { binary.LittleEndian.PutUint32(b[off+28:], 0x40000000) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := slices.Clone(b)
			tc.edit(data)
			m := ArtifactMetadata{}
			err := inspectEXE(installerFile(data), &m)
			if tc.fail != (err != nil) || (!tc.fail && (len(m.Targets) != 1 || m.Targets[0].Arch != "")) {
				t.Fatalf("%+v %v", m, err)
			}
		})
	}
	header := make([]byte, 68)
	binary.LittleEndian.PutUint32(header[20:], 68)
	binary.LittleEndian.PutUint32(header[24:], 0xffffffff)
	if _, err := nsisFiles(header); !errors.Is(err, ErrMalformedArtifact) {
		t.Fatalf("accepted invalid instruction count: %v", err)
	}
	plain := fixture(t, "go-windows-amd64.head")
	var m ArtifactMetadata
	if err := inspectEXE(installerFile(plain), &m); err != nil || len(m.Targets) != 1 || m.Targets[0].Arch != "amd64" {
		t.Fatalf("standalone EXE changed: %+v %v", m, err)
	}
}

func TestNSISStoredHeaderWithLZMAPropertyLength(t *testing.T) {
	le := binary.LittleEndian
	h := make([]byte, 0x15d)
	le.PutUint32(h[20:], 300)
	le.PutUint32(h[24:], 1)
	le.PutUint32(h[28:], 328)
	le.PutUint32(h[36:], 339)
	le.PutUint32(h[300:], 20)
	le.PutUint32(h[308:], 1)
	copy(h[328:], "\x00app-64.7z\x00")
	data := le.AppendUint32(nil, uint32(len(h)))
	data = append(data, h...)
	var m ArtifactMetadata
	m.Targets = []ArtifactTarget{{Platform: "windows"}}
	if err := inspectNSIS(installerFile(data), &m, 0, int64(len(data)), len(h)); err != nil || m.Targets[0].Arch != "amd64" {
		t.Fatalf("stored header mistaken for LZMA: %+v %v", m, err)
	}
}

func TestNSISFileRecordLimit(t *testing.T) {
	const count = 4097
	le := binary.LittleEndian
	h := make([]byte, 300+count*28+4)
	le.PutUint32(h[20:], 300)
	le.PutUint32(h[24:], count)
	le.PutUint32(h[28:], uint32(len(h)-4))
	le.PutUint32(h[36:], uint32(len(h)))
	for i := 0; i < count; i++ {
		le.PutUint32(h[300+i*28:], 20)
	}
	if _, err := nsisFiles(h); !errors.Is(err, errInspectLimit) {
		t.Fatalf("unbounded file records: %v", err)
	}
}

func TestNSISLargeStubUsesPEOverlay(t *testing.T) {
	b := nsisFixture(t, []string{"app-arm64.7z"}, false)
	old := bytes.Index(b, []byte("\xef\xbe\xad\xdeNullsoftInst")) - 4
	const overlay = inspectSpan + 512
	data := make([]byte, overlay)
	copy(data, b[:old])
	data = append(data, b[old:]...)
	le := binary.LittleEndian
	pe := le.Uint32(data[0x3c:])
	le.PutUint16(data[pe+6:], 1)
	section := pe + 24 + uint32(le.Uint16(data[pe+20:]))
	le.PutUint32(data[section+16:], overlay-512)
	le.PutUint32(data[section+20:], 512)
	s := newFakeDownloads(t)
	a := s.add("setup.exe", data)
	m, err := s.client().InspectArtifact(context.Background(), a)
	if err != nil || len(m.Targets) != 1 || m.Targets[0].Arch != "arm64" || m.Role != "installer" {
		t.Fatalf("large stub reported launcher CPU: %+v %v", m, err)
	}
	if s.requests > 2 {
		t.Fatalf("overlay needed %d requests", s.requests)
	}
}

func TestNSISSolidPayloadStaysWithinRangeBudget(t *testing.T) {
	b := nsisFixture(t, []string{"main.exe"}, true)
	off := bytes.Index(b, []byte("\xef\xbe\xad\xdeNullsoftInst")) - 4
	r, err := nsisLZMA(bytes.NewReader(b[off+28:]))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	// Later application data must not be read just to inspect its PE header.
	tail := make([]byte, 2<<20)
	rand.New(rand.NewSource(1)).Read(tail)
	var compressed bytes.Buffer
	w, err := (lzma.WriterConfig{DictCap: 8 << 20}).NewWriter(&compressed)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(append(decoded, tail...))
	w.Close()
	stream := compressed.Bytes()
	b = append(b[:off+28], append(append([]byte{}, stream[:5]...), stream[13:]...)...)
	binary.LittleEndian.PutUint32(b[off+24:], uint32(len(b)-off))
	s := newFakeDownloads(t)
	a := s.add("setup.exe", b)
	m, err := s.client().InspectArtifact(context.Background(), a)
	if err != nil || m.Status != "inspected" || len(m.Targets) != 1 || m.Targets[0].Arch != "arm64" {
		t.Fatalf("payload head exceeded budget: %+v %v", m, err)
	}
}
