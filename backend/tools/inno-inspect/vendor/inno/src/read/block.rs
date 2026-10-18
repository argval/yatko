use std::{cmp::min, io};

use zerocopy::LE;

use crate::{ReadBytesExt, entry::checksum::ChecksumMismatchError, error::InnoError};

pub const INNO_BLOCK_SIZE: u16 = 4 * 1024;

pub struct InnoBlockReader<R: io::Read> {
    /// The underlying reader.
    inner: R,

    /// The buffer for the block.
    buffer: [u8; INNO_BLOCK_SIZE as usize],

    /// The position of the reader within the current block.
    pos: usize,

    /// The length of the current block.
    ///
    /// This is always 4096, expect for the last block.
    length: usize,

    /// The total number of bytes read.
    total_in: usize,

    /// The total number of bytes produced.
    total_out: usize,
}

impl<R: io::Read> InnoBlockReader<R> {
    /// Creates a new `InnoBlockReader` from the given reader.
    #[must_use]
    pub const fn new(reader: R) -> Self {
        Self {
            inner: reader,
            buffer: [0; INNO_BLOCK_SIZE as usize],
            pos: 0,
            length: 0,
            total_in: 0,
            total_out: 0,
        }
    }

    /// Gets a reference to the underlying reader.
    ///
    /// It is inadvisable to directly read from the underlying reader.
    #[must_use]
    #[inline]
    pub const fn get_ref(&self) -> &R {
        &self.inner
    }

    /// Gets a mutable reference to the underlying reader.
    ///
    /// It is inadvisable to directly read from the underlying reader.
    #[must_use]
    #[inline]
    pub const fn get_mut(&mut self) -> &mut R {
        &mut self.inner
    }

    /// Consumes the chunk reader, returning the underlying reader.
    #[must_use]
    #[inline]
    pub fn into_inner(self) -> R {
        self.inner
    }

    /// Returns the number of bytes the block reader has read.
    #[must_use]
    #[inline]
    pub const fn total_in(&self) -> usize {
        self.total_in
    }

    /// Returns the number of bytes that the block reader has produced.
    #[must_use]
    #[inline]
    pub const fn total_out(&self) -> usize {
        self.total_out
    }

    fn read_block(&mut self) -> io::Result<bool> {
        let block_crc32 = match self.inner.read_u32::<LE>() {
            Ok(block_crc32) => block_crc32,
            Err(error) if error.kind() == io::ErrorKind::UnexpectedEof => return Ok(false),
            Err(error) => return Err(error),
        };

        self.total_in += size_of::<u32>();

        let mut length = 0;
        while length < self.buffer.len() {
            match self.inner.read(&mut self.buffer[length..]) {
                Ok(0) => break,
                Ok(read) => length += read,
                Err(error) if error.kind() == io::ErrorKind::Interrupted => {}
                Err(error) => return Err(error),
            }
        }

        self.total_in += length;

        if length == 0 {
            return Err(io::Error::new(
                io::ErrorKind::UnexpectedEof,
                "Unexpected Inno block end",
            ));
        }

        let actual_crc32 = crc32fast::hash(&self.buffer[..length]);

        if actual_crc32 != block_crc32 {
            return Err(io::Error::new(
                io::ErrorKind::InvalidData,
                InnoError::ChecksumMismatch {
                    location: "Inno block",
                    inner: ChecksumMismatchError::new_crc32(block_crc32, actual_crc32),
                },
            ));
        }

        self.length = length;
        self.pos = 0;

        Ok(true)
    }
}

impl<R: io::Read> io::Read for InnoBlockReader<R> {
    fn read(&mut self, dest: &mut [u8]) -> io::Result<usize> {
        let mut total_read = 0;

        while total_read < dest.len() {
            if self.pos == self.length && !self.read_block()? {
                break;
            }

            let to_copy = min(dest.len() - total_read, self.length - self.pos);

            dest[total_read..total_read + to_copy]
                .copy_from_slice(&self.buffer[self.pos..self.pos + to_copy]);

            self.pos += to_copy;
            total_read += to_copy;
        }

        self.total_out += total_read;

        Ok(total_read)
    }
}

#[cfg(test)]
mod tests {
    use std::io::Read;

    use super::{INNO_BLOCK_SIZE, InnoBlockReader};

    /// A reader that never returns bytes from two sides of a window
    /// boundary in one call, which is how a `BufReader` behaves: it hands
    /// back what its buffer currently holds and no more. `io::Read` allows
    /// this, and the default 8 KiB buffer means a 4 KiB block lands across a
    /// boundary as soon as the stream is not aligned to it, which the four
    /// checksum bytes in front of every block guarantee.
    struct WindowedReader {
        data: Vec<u8>,
        pos: usize,
        window: usize,
    }

    impl Read for WindowedReader {
        fn read(&mut self, buf: &mut [u8]) -> std::io::Result<usize> {
            let until_boundary = self.window - (self.pos % self.window);
            let take = buf
                .len()
                .min(until_boundary)
                .min(self.data.len() - self.pos);
            buf[..take].copy_from_slice(&self.data[self.pos..self.pos + take]);
            self.pos += take;
            Ok(take)
        }
    }

    impl WindowedReader {
        fn new(data: Vec<u8>) -> Self {
            Self {
                data,
                pos: 0,
                window: 8 * 1024,
            }
        }
    }

    /// Builds the on-disk form of a block: its CRC32, then its bytes.
    fn block(payload: &[u8]) -> Vec<u8> {
        let mut out = crc32fast::hash(payload).to_le_bytes().to_vec();
        out.extend_from_slice(payload);
        out
    }

    #[test]
    fn a_block_split_across_two_reads_is_still_read_whole() {
        // The second block is the one that straddles a boundary here, which
        // is why one block on its own never showed this.
        let first = vec![0xAB; INNO_BLOCK_SIZE as usize];
        let second = vec![0xCD; INNO_BLOCK_SIZE as usize];
        let mut data = block(&first);
        data.extend_from_slice(&block(&second));

        let mut reader = InnoBlockReader::new(WindowedReader::new(data));

        let mut out = Vec::new();
        reader.read_to_end(&mut out).expect("short reads are legal");

        assert_eq!(out.len(), first.len() + second.len());
        assert!(out[..first.len()].iter().all(|&b| b == 0xAB));
        assert!(out[first.len()..].iter().all(|&b| b == 0xCD));
    }

    #[test]
    fn a_short_final_block_still_ends_the_stream_cleanly() {
        // Filling the buffer must not turn a legitimately short last block
        // into an error: the fill stops at end of stream, not only when the
        // buffer is full.
        let first = vec![0xAB; INNO_BLOCK_SIZE as usize];
        let last = vec![0xCD; 100];
        let mut data = block(&first);
        data.extend_from_slice(&block(&last));

        let mut reader = InnoBlockReader::new(WindowedReader::new(data));

        let mut out = Vec::new();
        reader.read_to_end(&mut out).unwrap();

        assert_eq!(out.len(), first.len() + last.len());
        assert_eq!(reader.total_out(), first.len() + last.len());
    }

    #[test]
    fn a_block_that_fails_its_checksum_does_not_serve_its_bytes_anyway() {
        // A block is published only once its checksum agrees. Published
        // first, a failed block leaves a length that does not belong with
        // the position, and the next read either serves buffer contents
        // nobody validated or subtracts past zero working out how many of
        // them to serve.
        let first = vec![0xAB; INNO_BLOCK_SIZE as usize];
        let mut data = block(&first);
        let mut corrupt = block(&vec![0xCD; INNO_BLOCK_SIZE as usize]);
        corrupt[0] ^= 0xFF;
        data.extend_from_slice(&corrupt);

        let mut reader = InnoBlockReader::new(WindowedReader::new(data));

        let mut out = vec![0; INNO_BLOCK_SIZE as usize];
        reader
            .read_exact(&mut out)
            .expect("the first block is good");

        let mut more = [0; 16];
        assert!(
            reader.read(&mut more).is_err(),
            "the corrupt block must error"
        );
        assert_eq!(
            reader.total_out(),
            first.len(),
            "nothing from the corrupt block may be counted as produced"
        );

        // Reading on after the error is what the LZMA decoder above this
        // does, so it is not a hypothetical. The corrupt block was consumed
        // on the way to failing, so there is nothing left and this reports a
        // clean end; the point is that it reports at all.
        assert_eq!(reader.read(&mut more).unwrap(), 0);
    }
}
