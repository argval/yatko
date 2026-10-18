use std::{
    fmt,
    io::{self, Error, ErrorKind, Read, Take},
};

use flate2::read::ZlibDecoder;
use lzma_rust2::{Lzma2Reader, LzmaReader};
use zerocopy::{KnownLayout, TryFromBytes};

use crate::{
    ReadBytesExt,
    error::{InnoError, InnoResult},
    header::Compression,
    lzma_stream_header::LzmaStreamHeader,
    read::DataSource,
    read::chunk::Chunk,
};

/// Magic bytes at the start of each data chunk.
///
/// <https://github.com/jrsoftware/issrc/blob/is-6_7_3/Projects/Src/Shared.Struct.pas#L41>
#[derive(Clone, Copy, TryFromBytes, KnownLayout)]
#[repr(u32)]
pub enum ZlibID {
    Magic = u32::from_le_bytes(*b"zlb\x1a"),
}

impl ZlibID {
    pub fn try_read_from_io<R>(mut src: R) -> InnoResult<Self>
    where
        Self: Sized,
        R: Read,
    {
        let mut magic = [0; size_of::<Self>()];
        src.read_exact(&mut magic)?;
        Self::try_read_from_bytes(&magic).map_err(|_| InnoError::InvalidChunkMagic(magic))
    }

    /// Returns the magic as a static string slice.
    #[must_use]
    #[inline]
    pub const fn as_str(self) -> &'static str {
        "zlb\u{1a}"
    }
}

impl fmt::Display for ZlibID {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::Display::fmt(self.as_str(), f)
    }
}

/// A reader for a single data chunk in the Inno Setup data stream.
///
/// Data chunks use a different format from the header streams: they start with
/// a 4-byte magic (`zlb\x1a`) followed by raw compressed data (no CRC32-checked
/// block layer).
pub enum DataChunkReader<R: Read> {
    Stored(Take<R>),
    Zlib(ZlibDecoder<Take<R>>),
    Lzma1(Box<LzmaReader<Take<R>>>),
    Lzma2(Box<Lzma2Reader<Take<R>>>),
}

/// Read a 1-byte LZMA2 properties header and decode it into a raw dictionary
/// size in bytes, following the xz LZMA2 filter property encoding.
fn read_lzma2_dict_size<R: Read>(reader: &mut R) -> io::Result<u32> {
    let prop = reader.read_u8()?;
    if prop > 40 {
        return Err(Error::new(
            ErrorKind::InvalidData,
            "invalid LZMA2 dictionary size",
        ));
    }
    Ok(if prop == 40 {
        u32::MAX
    } else {
        let base = 2 | u32::from(prop & 1);
        base << (u32::from(prop) / 2 + 11)
    })
}

impl<R: DataSource> DataChunkReader<R> {
    /// Open a data chunk for reading.
    ///
    /// Positions the source at the chunk, which is `chunk.start_offset()`
    /// into the slice the chunk begins in, validates the chunk magic, and
    /// sets up the appropriate decompression.
    pub fn new(mut reader: R, chunk: &Chunk) -> InnoResult<Self> {
        if chunk.is_encrypted() {
            // We can't read encrypted chunks
            return Err(InnoError::Encrypted);
        }

        // Seek to the chunk position in the data stream
        reader.seek_to(chunk.first_slice(), chunk.start_offset())?;

        // Read and validate the magic
        ZlibID::try_read_from_io(&mut reader)?;

        // Limit reads to the compressed chunk size
        let mut chunk_reader = reader.take(chunk.original_size());

        match chunk.compression() {
            Compression::Stored => Ok(Self::Stored(chunk_reader)),
            Compression::Zlib => Ok(Self::Zlib(ZlibDecoder::new(chunk_reader))),
            Compression::LZMA1 => {
                let header = chunk_reader.read_t::<LzmaStreamHeader>()?;
                let lzma = LzmaReader::new_with_props(
                    chunk_reader,
                    u64::MAX,
                    header.props(),
                    header.dictionary_size(),
                    None,
                )
                .map_err(|err| Error::new(ErrorKind::InvalidData, err))?;
                Ok(Self::Lzma1(Box::new(lzma)))
            }
            Compression::LZMA2 => {
                let dict_size = read_lzma2_dict_size(&mut chunk_reader)?;
                Ok(Self::Lzma2(Box::new(Lzma2Reader::new(
                    chunk_reader,
                    dict_size,
                    None,
                ))))
            }
            other => Err(InnoError::UnsupportedCompression(other)),
        }
    }

    /// Gets a reference to the underlying reader.
    ///
    /// It is inadvisable to directly read from the underlying reader.
    #[must_use]
    pub fn get_ref(&self) -> &Take<R> {
        match self {
            Self::Stored(reader) => reader,
            Self::Zlib(reader) => reader.get_ref(),
            Self::Lzma1(reader) => reader.inner(),
            Self::Lzma2(reader) => reader.inner(),
        }
    }

    /// Gets a mutable reference to the underlying reader.
    ///
    /// It is inadvisable to directly read from the underlying reader.
    #[must_use]
    pub fn get_mut(&mut self) -> &mut Take<R> {
        match self {
            Self::Stored(reader) => reader,
            Self::Zlib(reader) => reader.get_mut(),
            Self::Lzma1(reader) => reader.inner_mut(),
            Self::Lzma2(reader) => reader.inner_mut(),
        }
    }

    /// Consumes this data chunk reader, returning the underlying reader.
    #[must_use]
    pub fn into_inner(self) -> Take<R> {
        match self {
            Self::Stored(reader) => reader,
            Self::Zlib(reader) => reader.into_inner(),
            Self::Lzma1(reader) => reader.into_inner(),
            Self::Lzma2(reader) => reader.into_inner(),
        }
    }
}

impl<R: Read> Read for DataChunkReader<R> {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        match self {
            Self::Stored(reader) => reader.read(buf),
            Self::Zlib(reader) => reader.read(buf),
            Self::Lzma1(reader) => reader.read(buf),
            Self::Lzma2(reader) => reader.read(buf),
        }
    }
}
