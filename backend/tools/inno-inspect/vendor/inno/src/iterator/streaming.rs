use std::{
    collections::VecDeque,
    fmt, io,
    io::{Read, Seek},
};

use flate2::{Decompress, FlushDecompress, Status};

#[cfg(test)]
use crate::entry::checksum::Checksum;
use crate::{
    Inno, Source,
    entry::{
        CompressionFilter,
        checksum::{ChecksumHasher, ChecksumMismatchError},
    },
    error::InnoResult,
    iterator::{ExtractEntry, files_reader::FilesReader},
    read::{Embedded, chunk::Chunk},
};

/// How much is read and filtered at a time. Has to be the filters' own block,
/// since their boundary rule is expressed in it.
const BLOCK: usize = CompressionFilter::BLOCK_SIZE;

fn new_block() -> Box<[u8; BLOCK]> {
    vec![0; BLOCK]
        .into_boxed_slice()
        .try_into()
        .unwrap_or_else(|_| unreachable!())
}

/// A checksum failure has to reach the caller through [`Read`], which carries
/// only an [`io::Error`].
pub(super) fn mismatch_to_io(error: ChecksumMismatchError) -> io::Error {
    io::Error::new(io::ErrorKind::InvalidData, error)
}

/// An iterator over matching files that hands out a reader for each, rather
/// than the file's bytes.
///
/// This is a lending iterator: the reader borrows the iterator, so only one can
/// exist at a time and [`Iterator`] cannot express it.
pub struct StreamingFiles<'reader, R: Read + Seek> {
    reader: FilesReader<'reader, R>,
    /// In the order the installer records them, which is the order a caller
    /// correlating results with anything else derived from that list expects.
    entries: VecDeque<ExtractEntry>,
    /// The chunk the reader is positioned in, so an entry in a different one
    /// re-points it rather than seeking inside the wrong chunk.
    chunk: Option<Chunk>,
    current_position: u64,
    block: Box<[u8; BLOCK]>,
    /// The part of `block` that has been read from the chunk but not yet handed
    /// to the caller.
    block_start: usize,
    block_end: usize,
}

impl<'reader, R: Read + Seek> StreamingFiles<'reader, R> {
    pub(crate) fn new<P>(inno: &'reader mut Inno<R>, mut predicate: P) -> Self
    where
        P: FnMut(&ExtractEntry) -> bool,
    {
        // Entry order, and not sorted into chunks: it is the only order a
        // caller can predict, so it is what a caller pairing these against the
        // file entries can rely on. Inno Setup writes each file's data into a
        // chunk as it walks the entries, so an offset within a chunk does not
        // decrease along this order and the reader still only moves forward.
        // A sequence rather than a set, because two entries can name one
        // location and a set keyed on order would drop the second.
        let mut entries = VecDeque::new();

        for (index, file) in inno.file_entries().iter().enumerate() {
            let Some(location) = inno.file_locations().get(file.location() as usize) else {
                continue;
            };

            let extract_entry = ExtractEntry::new(index, file.clone(), *location);

            if predicate(&extract_entry) {
                entries.push_back(extract_entry);
            }
        }

        let data_offset = inno
            .inner
            .setup_loader
            .data_offset()
            .try_into()
            .unwrap_or_else(|_| unreachable!());

        let source = match inno.slices.as_mut() {
            Some(slices) => Source::Slices(slices),
            None => Source::Embedded(Embedded::new(&mut inno.reader, data_offset)),
        };

        Self {
            reader: FilesReader::Source(Some(source)),
            entries,
            chunk: None,
            current_position: 0,
            block: new_block(),
            block_start: 0,
            block_end: 0,
        }
    }

    /// Builds an iterator that reads straight from `reader`, which stands in
    /// for the decompressed chunk, so that a [`FileReader`] can be exercised
    /// without an installer to open.
    #[cfg(test)]
    fn over(reader: &'reader mut R) -> Self {
        Self {
            reader: FilesReader::Source(Some(Source::Embedded(Embedded::new(reader, 0)))),
            entries: VecDeque::new(),
            chunk: None,
            current_position: 0,
            block: new_block(),
            block_start: 0,
            block_end: 0,
        }
    }

    /// Returns the number of entries still to be yielded.
    #[must_use]
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    /// Returns `true` if there are no entries left to yield.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    /// Advances to the next matching file, returning a reader over its bytes.
    ///
    /// The reader borrows this iterator, so the previous one must be dropped
    /// first. Whatever it left unread is skipped here.
    #[allow(clippy::should_implement_trait)]
    pub fn next(&mut self) -> Option<InnoResult<(ExtractEntry, FileReader<'_, 'reader, R>)>> {
        let entry = self.entries.pop_front()?;

        let file = entry.file_location().file();
        let target_offset = file.offset();
        let chunk = *entry.file_location().chunk();

        // Already counted in `current_position` when it left the chunk.
        self.block_start = 0;
        self.block_end = 0;

        // A chunk cannot seek backwards and the reader may be in a different
        // one, so either case starts that chunk again. Entry order does not go
        // backwards within a chunk on an installer Inno Setup built, so this is
        // one pass per chunk in practice rather than a rewind per entry.
        if self.chunk != Some(chunk) || target_offset < self.current_position {
            if let Err(err) = self.reader.reinitialize(&chunk) {
                return Some(Err(err));
            }

            self.chunk = Some(chunk);
            self.current_position = 0;
        }

        if self.current_position < target_offset {
            if let Err(err) = io::copy(
                &mut self
                    .reader
                    .by_ref()
                    .take(target_offset - self.current_position),
                &mut io::sink(),
            ) {
                return Some(Err(err.into()));
            }

            self.current_position = target_offset;
        }

        let filter = file.compression_filter();

        let reader = FileReader {
            // A `ZlibFilter` location is a nested zlib stream rather than a
            // transform of the block, so it is inflated instead of filtered.
            inflate: filter.is_zlib().then(|| Box::new(Decompress::new(true))),
            inflate_ended: false,
            filter,
            hasher: Some(file.checksum().hasher()),
            position: 0,
            location_remaining: file.size(),
            carry: 0,
            files: self,
        };

        Some(Ok((entry, reader)))
    }

    /// Fills `block` from `at` onwards with the next of the location, at most
    /// `limit` bytes, returning how many arrived.
    fn fill_block(&mut self, limit: u64, at: usize) -> io::Result<usize> {
        let count = usize::try_from(limit.min((BLOCK - at) as u64)).unwrap_or(BLOCK - at);

        self.reader.read_exact(&mut self.block[at..at + count])?;
        self.current_position += count as u64;
        self.block_start = 0;
        self.block_end = at + count;

        Ok(count)
    }
}

impl<R: Read + Seek> fmt::Debug for StreamingFiles<'_, R> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("StreamingFiles")
            .field("remaining", &self.len())
            .field("current_position", &self.current_position)
            .finish_non_exhaustive()
    }
}

/// A reader over one extracted file, filtering and checksumming its bytes as
/// they pass through a fixed block.
pub struct FileReader<'files, 'reader, R: Read + Seek> {
    files: &'files mut StreamingFiles<'reader, R>,
    filter: CompressionFilter,
    /// Set only for a [`CompressionFilter::ZlibFilter`] location.
    inflate: Option<Box<Decompress>>,
    /// Taken at the end of the location, so its absence marks that the checksum
    /// has been verified and nothing more will be read.
    hasher: Option<ChecksumHasher>,
    /// Offset of the next byte within the file: the instruction filters encode
    /// addresses relative to it. Unused on the zlib path.
    position: u64,
    /// Bytes of the location left to take from the chunk.
    location_remaining: u64,
    /// Whether the zlib stream reported its end alongside its final bytes.
    inflate_ended: bool,
    /// Held back from the last block, undecodable until what follows arrives.
    carry: usize,
}

impl<R: Read + Seek> FileReader<'_, '_, R> {
    fn finish(&mut self) -> io::Result<usize> {
        if let Some(hasher) = self.hasher.take() {
            hasher.finish().map_err(mismatch_to_io)?;
        }

        Ok(0)
    }

    fn read_filtered(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        if self.files.block_start == self.files.block_end {
            if self.location_remaining == 0 {
                return self.finish();
            }

            let files = &mut *self.files;

            // The carry goes first, so the scan resumes where it stopped.
            files
                .block
                .copy_within(files.block_end..files.block_end + self.carry, 0);

            let count = files.fill_block(self.location_remaining, self.carry)?;
            self.location_remaining -= count as u64;

            let filled = self.carry + count;
            let decoded = self.filter.decode_block(
                &mut files.block[..filled],
                self.position,
                self.location_remaining == 0,
            );

            self.carry = filled - decoded;
            files.block_end = decoded;
            self.position += decoded as u64;

            if let Some(hasher) = &mut self.hasher {
                hasher.update(&files.block[..decoded]);
            }
        }

        let files = &mut *self.files;
        let count = buf.len().min(files.block_end - files.block_start);
        buf[..count].copy_from_slice(&files.block[files.block_start..files.block_start + count]);
        files.block_start += count;

        Ok(count)
    }

    fn read_inflated(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        if self.inflate_ended {
            return self.finish();
        }

        loop {
            if self.files.block_start == self.files.block_end && self.location_remaining > 0 {
                let count = self.files.fill_block(self.location_remaining, 0)?;
                self.location_remaining -= count as u64;
            }

            let files = &mut *self.files;
            let Some(inflate) = self.inflate.as_mut() else {
                unreachable!()
            };

            let (read_before, written_before) = (inflate.total_in(), inflate.total_out());
            let status = inflate
                .decompress(
                    &files.block[files.block_start..files.block_end],
                    buf,
                    FlushDecompress::None,
                )
                .map_err(|err| io::Error::new(io::ErrorKind::InvalidData, err))?;

            let read = (inflate.total_in() - read_before) as usize;
            let count = (inflate.total_out() - written_before) as usize;
            files.block_start += read;

            if count > 0 {
                // The stream can end on the same call that returns its last
                // bytes, and a caller always reads once more after that. Which
                // branch that read takes cannot be left to the decoder: zlib
                // reports `StreamEnd` again, and miniz_oxide, which is
                // flate2's default backend, returns nothing at all and would
                // fall to the truncation check below.
                self.inflate_ended = matches!(status, Status::StreamEnd);

                if let Some(hasher) = &mut self.hasher {
                    hasher.update(&buf[..count]);
                }
                self.position += count as u64;

                return Ok(count);
            }

            if matches!(status, Status::StreamEnd) {
                break;
            }

            // Room in the buffer and bytes in the block, yet nothing moved:
            // the stream is cut short.
            if read == 0 {
                return Err(io::ErrorKind::UnexpectedEof.into());
            }
        }

        self.finish()
    }
}

impl<R: Read + Seek> Read for FileReader<'_, '_, R> {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        // An empty `buf` would make `read_inflated` see every call as a
        // truncated stream.
        if buf.is_empty() || self.hasher.is_none() {
            return Ok(0);
        }

        if self.inflate.is_some() {
            self.read_inflated(buf)
        } else {
            self.read_filtered(buf)
        }
    }
}

#[cfg(test)]
impl<'files, 'reader, R: Read + Seek> FileReader<'files, 'reader, R> {
    /// Reads a location of `size` bytes from `files`, as
    /// [`StreamingFiles::next`] would have set one up.
    fn over(
        files: &'files mut StreamingFiles<'reader, R>,
        filter: CompressionFilter,
        checksum: Checksum,
        size: u64,
    ) -> Self {
        Self {
            inflate: filter.is_zlib().then(|| Box::new(Decompress::new(true))),
            inflate_ended: false,
            filter,
            hasher: Some(checksum.hasher()),
            position: 0,
            location_remaining: size,
            carry: 0,
            files,
        }
    }
}

impl<R: Read + Seek> fmt::Debug for FileReader<'_, '_, R> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("FileReader")
            .field("filter", &self.filter)
            .field("position", &self.position)
            .field("location_remaining", &self.location_remaining)
            .field("hasher", &self.hasher)
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use std::io::{self, Cursor, Read};

    use super::{Checksum, CompressionFilter, FileReader, StreamingFiles};

    /// A checksum failure has to reach the caller through `Read`, which only
    /// carries `io::Error`.
    #[test]
    fn a_checksum_mismatch_becomes_invalid_data() {
        let mut hasher = Checksum::new_crc32(crc32fast::hash(b"expected")).hasher();
        hasher.update(b"actual");
        let error = super::mismatch_to_io(hasher.finish().unwrap_err());
        assert_eq!(error.kind(), io::ErrorKind::InvalidData);
    }

    /// The contract that separates this from `filtered_files`: bytes are handed
    /// over before they are known to be sound, so a mismatch arrives last.
    #[test]
    fn a_corrupt_location_reads_its_bytes_and_then_fails() {
        const DATA: &[u8] = b"the quick brown fox jumps over the lazy dog";

        let mut chunk = Cursor::new(DATA.to_vec());
        let mut files = StreamingFiles::over(&mut chunk);
        let mut reader = FileReader::over(
            &mut files,
            CompressionFilter::NoFilter,
            Checksum::new_crc32(crc32fast::hash(b"something else entirely")),
            DATA.len() as u64,
        );

        let mut buf = [0u8; 64];
        let count = reader.read(&mut buf).expect("the bytes arrive");
        assert_eq!(&buf[..count], DATA);

        let error = reader.read(&mut buf).expect_err("the mismatch arrives");
        assert_eq!(error.kind(), io::ErrorKind::InvalidData);
    }

    /// A truncated location must not look like a checksum mismatch.
    #[test]
    fn a_truncated_location_fails_differently() {
        const DATA: &[u8] = b"not as much as promised";

        let mut chunk = Cursor::new(DATA.to_vec());
        let mut files = StreamingFiles::over(&mut chunk);
        let mut reader = FileReader::over(
            &mut files,
            CompressionFilter::NoFilter,
            Checksum::new_crc32(crc32fast::hash(DATA)),
            DATA.len() as u64 + 1,
        );

        let error = io::copy(&mut reader, &mut io::sink()).expect_err("the read is cut short");
        assert_eq!(error.kind(), io::ErrorKind::UnexpectedEof);
    }

    /// An entry of no bytes still has a checksum, of nothing.
    #[test]
    fn a_zero_length_entry_reads_nothing_and_validates() {
        let mut chunk = Cursor::new(Vec::new());
        let mut files = StreamingFiles::over(&mut chunk);
        let mut reader = FileReader::over(
            &mut files,
            CompressionFilter::NoFilter,
            Checksum::new_crc32(crc32fast::hash(b"")),
            0,
        );

        let mut data = Vec::new();
        assert_eq!(reader.read_to_end(&mut data).expect("read"), 0);
        assert!(data.is_empty());
    }

    /// Or the zero-length case would be a hole in the checking.
    #[test]
    fn a_zero_length_entry_with_a_wrong_checksum_fails() {
        let mut chunk = Cursor::new(Vec::new());
        let mut files = StreamingFiles::over(&mut chunk);
        let mut reader = FileReader::over(
            &mut files,
            CompressionFilter::NoFilter,
            Checksum::new_crc32(crc32fast::hash(b"not nothing")),
            0,
        );

        let error = io::copy(&mut reader, &mut io::sink()).expect_err("the mismatch arrives");
        assert_eq!(error.kind(), io::ErrorKind::InvalidData);
    }
}
