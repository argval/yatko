package github

import (
	"archive/zip"
	"encoding/binary"
	"io"
)

const (
	zipMaxRecords        = 4096
	zipMaxDirectoryBytes = 1 << 20
)

// boundedZIPReader checks classic and ZIP64 directory metadata before archive/zip
// can allocate from its entry count. Nothing is extracted during this check.
func boundedZIPReader(r io.ReaderAt, size int64) (*zip.Reader, error) {
	if size < 22 {
		return nil, malformed("ZIP end record")
	}
	n := min(size, int64(archiveTailLimit))
	tail := make([]byte, int(n))
	if _, err := r.ReadAt(tail, size-n); err != nil {
		return nil, err
	}
	le := binary.LittleEndian
	end := -1
	for i := len(tail) - 22; i >= 0; i-- {
		if le.Uint32(tail[i:]) == 0x06054b50 && i+22+int(le.Uint16(tail[i+20:])) == len(tail) {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, malformed("ZIP end/comment bounds")
	}
	e := tail[end:]
	eo := size - n + int64(end)
	if le.Uint16(e[4:]) != 0 || le.Uint16(e[6:]) != 0 {
		return nil, errInspectLimit
	}
	recordsHere, records := uint64(le.Uint16(e[8:])), uint64(le.Uint16(e[10:]))
	dirSize, dirOff := uint64(le.Uint32(e[12:])), uint64(le.Uint32(e[16:]))
	dirEnd := eo
	if records == 0xffff || recordsHere == 0xffff || dirSize == 0xffffffff || dirOff == 0xffffffff {
		if eo < 20 {
			return nil, malformed("ZIP64 locator")
		}
		var loc [20]byte
		if _, err := r.ReadAt(loc[:], eo-20); err != nil {
			return nil, err
		}
		if le.Uint32(loc[:]) != 0x07064b50 {
			return nil, malformed("ZIP64 locator signature")
		}
		if le.Uint32(loc[4:]) != 0 || le.Uint32(loc[16:]) != 1 {
			return nil, errInspectLimit
		}
		off := le.Uint64(loc[8:])
		if eo < 76 || off > uint64(eo-76) {
			return nil, malformed("ZIP64 end offset")
		}
		var z [56]byte
		if _, err := r.ReadAt(z[:], int64(off)); err != nil {
			return nil, err
		}
		if le.Uint32(z[:]) != 0x06064b50 {
			return nil, malformed("ZIP64 end signature")
		}
		length := le.Uint64(z[4:])
		if length < 44 || length != uint64(eo-20)-off-12 {
			return nil, malformed("ZIP64 end size")
		}
		if le.Uint32(z[16:]) != 0 || le.Uint32(z[20:]) != 0 {
			return nil, errInspectLimit
		}
		if (recordsHere != 0xffff && recordsHere != le.Uint64(z[24:])) ||
			(records != 0xffff && records != le.Uint64(z[32:])) ||
			(dirSize != 0xffffffff && dirSize != le.Uint64(z[40:])) ||
			(dirOff != 0xffffffff && dirOff != le.Uint64(z[48:])) {
			return nil, malformed("ZIP64/classic directory disagreement")
		}
		recordsHere, records = le.Uint64(z[24:]), le.Uint64(z[32:])
		dirSize, dirOff = le.Uint64(z[40:]), le.Uint64(z[48:])
		dirEnd = int64(off)
	}
	if recordsHere != records {
		return nil, malformed("ZIP directory entry counts")
	}
	if records > zipMaxRecords || dirSize > zipMaxDirectoryBytes {
		return nil, errInspectLimit
	}
	if dirSize > uint64(dirEnd) || dirOff > uint64(dirEnd)-dirSize || records > dirSize/46 {
		return nil, malformed("ZIP directory bounds")
	}
	dirStart := dirEnd - int64(dirSize)
	// Hide the comment while archive/zip finds the end record. Otherwise a quoted
	// EOCD signature in that comment could select different, unchecked counts.
	view := &zipDirectoryView{r: r, start: dirStart, end: eo + 22, commentLengthAt: eo + 20, initializing: true}
	zr, err := zip.NewReader(view, eo+22)
	view.initializing = false
	if err != nil {
		return nil, err
	}
	if len(zr.File) > zipMaxRecords {
		return nil, errInspectLimit
	}
	if uint64(len(zr.File)) != records {
		return nil, malformed("ZIP directory entry count")
	}
	zr.Comment = string(e[22:])
	return zr, nil
}

// zipDirectoryView confines initial directory parsing to its validated interval.
// After initialization, entry readers can access the original asset normally.
type zipDirectoryView struct {
	r                           io.ReaderAt
	start, end, commentLengthAt int64
	initializing                bool
}

func (v *zipDirectoryView) ReadAt(p []byte, off int64) (int, error) {
	if v.initializing && (off < 0 || off >= v.end) {
		return 0, io.EOF
	}
	requested := len(p)
	if v.initializing && int64(len(p)) > v.end-off {
		p = p[:v.end-off]
	}
	prefix := 0
	if v.initializing && off < v.start {
		prefix = int(min(int64(len(p)), v.start-off))
		if prefix == len(p) {
			return 0, io.EOF
		}
		clear(p[:prefix])
	}
	n, err := v.r.ReadAt(p[prefix:], off+int64(prefix))
	n += prefix
	for i := int64(0); i < 2; i++ {
		pos := v.commentLengthAt + i - off
		if pos >= 0 && pos < int64(n) {
			p[pos] = 0
		}
	}
	if n < requested && err == nil {
		err = io.EOF
	}
	return n, err
}
