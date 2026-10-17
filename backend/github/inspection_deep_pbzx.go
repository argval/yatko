package github

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ulikunitz/xz"
)

// pbzxMaxChunk bounds one compressed chunk; Apple writes 16 MiB chunks.
const pbzxMaxChunk = 64 << 20

var xzMagic = []byte{0xfd, '7', 'z', 'X', 'Z', 0}

// isPBZX reports whether a staged file is an Apple pbzx stream, the format of
// flat-package Payload files. 7-Zip reads only its first xz chunk.
func isPBZX(filename string) bool {
	f, err := os.Open(filename)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4)
	_, err = io.ReadFull(f, head)
	return err == nil && string(head) == "pbzx"
}

// decodePBZX writes the cpio archive inside a pbzx stream to a new staged
// file. The stream is "pbzx", a flags word, then chunks of (flags, length,
// data) while bit 24 of the previous flags is set; a chunk is xz unless it
// lacks the xz magic, in which case it is stored. Output counts toward the
// session's decoded-byte budget.
func (s *deepSession) decodePBZX(filename string) (string, error) {
	in, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer in.Close()
	r := bufio.NewReader(in)
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil || string(hdr[:4]) != "pbzx" {
		return "", malformed("pbzx header")
	}
	out, err := os.CreateTemp(s.dir, "pbzx-*.cpio")
	if err != nil {
		return "", err
	}
	defer out.Close()
	w := &deepFileWriter{file: out, limit: min(deepMaxDownload, deepMaxStaged-s.staged)}
	defer func() { s.staged += w.written }()
	flags := binary.BigEndian.Uint64(hdr[4:])
	for flags&(1<<24) != 0 {
		if err := s.ctx.Err(); err != nil {
			return "", err
		}
		var ch [16]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return "", malformed("pbzx chunk header")
		}
		flags = binary.BigEndian.Uint64(ch[:8])
		length := binary.BigEndian.Uint64(ch[8:])
		if length == 0 || length > pbzxMaxChunk {
			return "", malformed("pbzx chunk length")
		}
		chunk := make([]byte, length)
		if _, err := io.ReadFull(r, chunk); err != nil {
			return "", malformed("pbzx chunk data")
		}
		if !bytes.HasPrefix(chunk, xzMagic) {
			if _, err := w.Write(chunk); err != nil {
				return "", err
			}
			continue
		}
		xr, err := xz.NewReader(bytes.NewReader(chunk))
		if err != nil {
			return "", fmt.Errorf("%w: pbzx xz chunk: %v", ErrMalformedArtifact, err)
		}
		if _, err := io.Copy(w, xr); err != nil {
			if errors.Is(err, errInspectLimit) {
				return "", err
			}
			return "", fmt.Errorf("%w: pbzx xz chunk: %v", ErrMalformedArtifact, err)
		}
	}
	return out.Name(), nil
}
