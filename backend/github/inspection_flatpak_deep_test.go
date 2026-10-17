package github

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"slices"
	"testing"
)

// These small superblocks were serialized by GLib, independent of the Go reader.
var flatpakFixtureHex = map[string]string{
	"app-big": "666c617470616b00e58900010075080072656600000000006170702f6f72672e6578616d706c652e4170702f7838365f36342f737461626c65000073040000006d6574616461746100000000000000005b4170706c69636174696f6e5d0a6e616d653d6f72672e6578616d706c652e4170700a72756e74696d653d6f72672e6578616d706c652e506c6174666f726d2f7838365f36342f310a73646b3d6f72672e6578616d706c652e53646b2f7838365f36342f310a000073090f3dba000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000010020100000000000001fe00fe00e800c800bd00",
	"app":     "666c617470616b00010089e50075080072656600000000006170702f6f72672e6578616d706c652e4170702f7838365f36342f737461626c65000073040000006d6574616461746100000000000000005b4170706c69636174696f6e5d0a6e616d653d6f72672e6578616d706c652e4170700a72756e74696d653d6f72672e6578616d706c652e506c6174666f726d2f7838365f36342f310a73646b3d6f72672e6578616d706c652e53646b2f7838365f36342f310a000073090f3dba000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000010020100000000000001fe00fe00e800c800bd00",
	"runtime": "666c617470616b00010089e500750800726566000000000072756e74696d652f6f72672e6578616d706c652e506c6174666f726d2f616172636836342f737461626c6500007304006d6574616461746100000000000000005b52756e74696d655d0a6e616d653d6f72672e6578616d706c652e506c6174666f726d0a000073090f4780000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001002010000000000c8c6c6b09083",
	"tiny":    "666c617470616b00010089e50075080072656600000000006170702f6f72672e6578616d706c652e4170702f61726d2f737461626c65000073040f3a0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001002010000000000807e7e68483c",
}

func TestStructuredFlatpakBundles(t *testing.T) {
	for _, tc := range []struct{ name, arch, role string }{
		{"app", "amd64", "application"},
		{"app-big", "amd64", "application"},
		{"runtime", "arm64", "metadata"},
		{"tiny", "arm", "application"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := hex.DecodeString(flatpakFixtureHex[tc.name])
			if err != nil {
				t.Fatal(err)
			}
			r := &peReadCounter{ReaderAt: bytes.NewReader(data)}
			m := ArtifactMetadata{Role: "unknown"}
			if err := inspectFlatpakFile(r, int64(len(data)), &m); err != nil || m.Role != tc.role || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "linux", Arch: tc.arch}}) || len(m.Conflicts) != 0 {
				t.Fatalf("metadata=%+v err=%v", m, err)
			}
			if r.bytes > 4096 {
				t.Fatalf("unbounded metadata reads: %d", r.bytes)
			}
		})
	}
}

func TestFlatpakRefMetadataConflicts(t *testing.T) {
	base, err := hex.DecodeString(flatpakFixtureHex["app"])
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		old, new string
		all      bool
		arch     string
		conflict bool
	}{
		{"ref CPU mismatch", "x86_64", "mips64", false, "", true},
		{"SDK CPU mismatch", "Sdk/x86_64", "Sdk/mips64", false, "", true},
		{"name mismatch", "name=org.example.App", "name=org.example.Bad", false, "", true},
		{"unsupported CPU stays explicit", "x86_64", "mips64", true, "flatpak:mips64", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 1
			if tc.all {
				count = -1
			}
			data := bytes.Replace(base, []byte(tc.old), []byte(tc.new), count)
			if len(data) != len(base) || bytes.Equal(data, base) {
				t.Fatal("fixture replacement failed")
			}
			m := ArtifactMetadata{Role: "unknown"}
			if err := inspectFlatpakFile(bytes.NewReader(data), int64(len(data)), &m); err != nil || !slices.Equal(m.Targets, []ArtifactTarget{{Platform: "linux", Arch: tc.arch}}) || (len(m.Conflicts) > 0) != tc.conflict {
				t.Fatalf("metadata=%+v err=%v", m, err)
			}
		})
	}
}

func TestFlatpakRejectsUnframedAndMalformedMetadata(t *testing.T) {
	base, err := hex.DecodeString(flatpakFixtureHex["app"])
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func([]byte) []byte
	}{
		{"plain strings are not bundle evidence", func(b []byte) []byte {
			return []byte("flatpak\x00app/org.example.App/x86_64/stable\x00[Application]\nname=org.example.App\n")
		}},
		{"truncated tuple", func(b []byte) []byte { return b[:len(b)-1] }},
		{"bad magic", func(b []byte) []byte { b[8] = 2; return b }},
		{"unterminated key", func(b []byte) []byte { b[7] = 'x'; return b }},
		{"nonzero key alignment padding", func(b []byte) []byte { b[20] = 1; return b }},
		{"wrong ref variant type", func(b []byte) []byte {
			return bytes.Replace(b, []byte("stable\x00\x00s"), []byte("stable\x00\x00u"), 1)
		}},
		{"dict offset outside dictionary", func(b []byte) []byte { b[188] = 255; return b }},
		{"tuple metadata outside body", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[len(b)-2:], 65535); return b }},
		{"tuple offsets run backwards", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[len(b)-4:], 1); return b }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.edit(slices.Clone(base))
			r := &peReadCounter{ReaderAt: bytes.NewReader(data)}
			m := ArtifactMetadata{Role: "unknown"}
			err := inspectFlatpakFile(r, int64(len(data)), &m)
			if !errors.Is(err, ErrMalformedArtifact) || len(m.Targets) != 0 || r.bytes > 4096 {
				t.Fatalf("metadata=%+v err=%v read=%d", m, err, r.bytes)
			}
		})
	}
}

func TestFlatpakDictionaryEntryLimit(t *testing.T) {
	data := make([]byte, 11000)
	for i, end := range []uint16{10000, 10008, 10040, 10041, 10041, 10048} {
		binary.LittleEndian.PutUint16(data[len(data)-2*(i+1):], end)
	}
	m := ArtifactMetadata{Role: "unknown"}
	r := &peReadCounter{ReaderAt: bytes.NewReader(data)}
	if err := inspectFlatpakFile(r, int64(len(data)), &m); !errors.Is(err, errInspectLimit) || len(m.Targets) != 0 || r.bytes > 32 {
		t.Fatalf("metadata=%+v err=%v read=%d", m, err, r.bytes)
	}
}
