package github

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func safetyZIP(t *testing.T, comment string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	if err := zw.SetComment(comment); err != nil {
		t.Fatal(err)
	}
	w, err := zw.Create("app/Contents/MacOS/app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// safetyZIP64 replaces only the end structures, keeping the small directory real.
func safetyZIP64(t *testing.T) []byte {
	b := safetyZIP(t, "")
	le := binary.LittleEndian
	e := b[len(b)-22:]
	off := len(b) - 22
	z := make([]byte, 56+20+22)
	le.PutUint32(z, 0x06064b50)
	le.PutUint64(z[4:], 44)
	le.PutUint16(z[12:], 45)
	le.PutUint16(z[14:], 45)
	le.PutUint64(z[24:], uint64(le.Uint16(e[8:])))
	le.PutUint64(z[32:], uint64(le.Uint16(e[10:])))
	le.PutUint64(z[40:], uint64(le.Uint32(e[12:])))
	le.PutUint64(z[48:], uint64(le.Uint32(e[16:])))
	le.PutUint32(z[56:], 0x07064b50)
	le.PutUint64(z[64:], uint64(off))
	le.PutUint32(z[72:], 1)
	copy(z[76:], e)
	le.PutUint16(z[84:], 0xffff)
	le.PutUint16(z[86:], 0xffff)
	le.PutUint32(z[88:], 0xffffffff)
	le.PutUint32(z[92:], 0xffffffff)
	return append(b[:off], z...)
}

func TestBoundedZIPReaderValid(t *testing.T) {
	comment := "quoted signature: PK\x05\x06" + strings.Repeat("\x00", 18) + " suffix"
	for _, b := range [][]byte{safetyZIP(t, ""), safetyZIP(t, comment), safetyZIP(t, strings.Repeat("x", 65535)), safetyZIP64(t), append([]byte("self extracting prefix"), safetyZIP(t, "")...)} {
		zr, err := boundedZIPReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Fatal(err)
		}
		if len(zr.File) != 1 {
			t.Fatalf("files = %d", len(zr.File))
		}
		rc, err := zr.File[0].Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil || string(body) != "payload" {
			t.Fatalf("body=%q err=%v", body, err)
		}
	}
	zr, err := boundedZIPReader(bytes.NewReader(safetyZIP(t, comment)), int64(len(safetyZIP(t, comment))))
	if err != nil || zr.Comment != comment {
		t.Fatalf("comment lost: %v", err)
	}
	var empty bytes.Buffer
	zw := zip.NewWriter(&empty)
	zw.Close()
	if zr, err := boundedZIPReader(bytes.NewReader(empty.Bytes()), int64(empty.Len())); err != nil || len(zr.File) != 0 {
		t.Fatalf("empty ZIP: %v", err)
	}
}

func TestBoundedZIPReaderMalformedAndLimits(t *testing.T) {
	le := binary.LittleEndian
	for _, tc := range []struct {
		name   string
		mutate func([]byte)
		limit  bool
	}{
		{"record limit", func(e []byte) { le.PutUint16(e[8:], 4097); le.PutUint16(e[10:], 4097) }, true},
		{"directory byte limit", func(e []byte) { le.PutUint32(e[12:], zipMaxDirectoryBytes+1) }, true},
		{"multi disk", func(e []byte) { le.PutUint16(e[4:], 1) }, true},
		{"count mismatch", func(e []byte) { le.PutUint16(e[8:], 0) }, false},
		{"directory offset", func(e []byte) { le.PutUint32(e[16:], 0xfffffffe) }, false},
		{"comment bounds", func(e []byte) { le.PutUint16(e[20:], 1) }, false},
		{"record size contradiction", func(e []byte) { le.PutUint32(e[12:], 0) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := safetyZIP(t, "")
			tc.mutate(b[len(b)-22:])
			_, err := boundedZIPReader(bytes.NewReader(b), int64(len(b)))
			if err == nil || errors.Is(err, errInspectLimit) != tc.limit {
				t.Fatalf("error = %v", err)
			}
		})
	}
	b := safetyZIP64(t)
	le.PutUint64(b[len(b)-34:], ^uint64(0))
	if _, err := boundedZIPReader(bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("accepted huge ZIP64 pointer")
	}
	b = safetyZIP64(t)
	le.PutUint64(b[len(b)-98+4:], 43)
	if _, err := boundedZIPReader(bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("accepted short ZIP64 record")
	}
	b = safetyZIP64(t)
	le.PutUint16(b[len(b)-22+8:], 2)
	if _, err := boundedZIPReader(bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("accepted conflicting classic and ZIP64 counts")
	}
	if _, err := boundedZIPReader(bytes.NewReader([]byte("PK")), 2); err == nil {
		t.Fatal("accepted truncated ZIP")
	}
}

// virtualZIP exposes a huge advertised file using only a small in-memory tail.
type virtualZIP struct {
	size           int64
	tail           []byte
	calls, maxRead int
}

func (v *virtualZIP) ReadAt(p []byte, off int64) (int, error) {
	v.calls++
	v.maxRead = max(v.maxRead, len(p))
	if off < v.size-int64(len(v.tail)) || off >= v.size {
		return 0, io.EOF
	}
	n := copy(p, v.tail[off-(v.size-int64(len(v.tail))):])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestBoundedZIPReaderRejectsHugeZIP64BeforeAllocation(t *testing.T) {
	v := &virtualZIP{size: 1 << 40, tail: make([]byte, archiveTailLimit)}
	z := v.tail[len(v.tail)-98:]
	le := binary.LittleEndian
	le.PutUint32(z, 0x06064b50)
	le.PutUint64(z[4:], 44)
	le.PutUint64(z[24:], 1<<32)
	le.PutUint64(z[32:], 1<<32)
	le.PutUint64(z[40:], 46)
	le.PutUint32(z[56:], 0x07064b50)
	le.PutUint64(z[64:], uint64(v.size-98))
	le.PutUint32(z[72:], 1)
	le.PutUint32(z[76:], 0x06054b50)
	le.PutUint16(z[84:], 0xffff)
	le.PutUint16(z[86:], 0xffff)
	le.PutUint32(z[88:], 0xffffffff)
	le.PutUint32(z[92:], 0xffffffff)
	if _, err := boundedZIPReader(v, v.size); !errors.Is(err, errInspectLimit) {
		t.Fatalf("huge count = %v", err)
	}
	if v.calls > 3 || v.maxRead > archiveTailLimit {
		t.Fatalf("unbounded reads: %+v", v)
	}
}

// Exercise the public readers too, so preflight cannot be disconnected silently.
func TestZIPInspectorsEnforceRecordLimit(t *testing.T) {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for range zipMaxRecords + 1 {
		if _, err := zw.Create("padding.txt"); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	for name, inspect := range map[string]func(*remoteFile, *ArtifactMetadata) error{"zip": inspectZIP, "apk": inspectAPK} {
		t.Run(name, func(t *testing.T) {
			if err := inspect(installerFile(b.Bytes()), &ArtifactMetadata{}); !errors.Is(err, errInspectLimit) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
