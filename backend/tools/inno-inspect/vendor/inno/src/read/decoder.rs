use std::io;

use flate2::read::ZlibDecoder;
use lzma_rust2::LzmaReader;

use crate::LzmaStreamHeader;

pub enum Decoder<R: io::Read> {
    Stored(R),
    Zlib(ZlibDecoder<R>),
    Lzma1(Box<LzmaReader<R>>),
}

impl<R: io::Read> Decoder<R> {
    /// Creates a new LZMA1 decoder from a reader and an [`LzmaStreamHeader`].
    pub fn new_lzma1(reader: R, header: LzmaStreamHeader) -> lzma_rust2::Result<Self> {
        LzmaReader::new_with_props(
            reader,
            u64::MAX,
            header.props(),
            header.dictionary_size(),
            None,
        )
        .map(|reader| Self::Lzma1(Box::new(reader)))
    }

    /// Creates a new Zlib decoder from a reader.
    pub fn new_zlib(reader: R) -> Self {
        Self::Zlib(ZlibDecoder::new(reader))
    }

    /// Returns `true` if the underlying stream is compressed.
    #[must_use]
    #[inline]
    pub const fn is_compressed(&self) -> bool {
        !self.is_stored()
    }

    /// Returns `true` if the stream is "stored". I.e, it is not compressed.
    #[must_use]
    #[inline]
    pub const fn is_stored(&self) -> bool {
        matches!(self, Self::Stored(_))
    }

    /// Returns `true` if the stream is compressed with zlib.
    #[must_use]
    #[inline]
    pub const fn is_zlib(&self) -> bool {
        matches!(self, Self::Zlib(_))
    }

    /// Returns `true` if the stream is compressed with LZMA 1.
    #[must_use]
    #[inline]
    pub const fn is_lzma1(&self) -> bool {
        matches!(self, Self::Lzma1(_))
    }

    /// Gets a reference to the underlying reader.
    ///
    /// It is inadvisable to directly read from the underlying reader.
    #[must_use]
    pub fn get_ref(&self) -> &R {
        match self {
            Self::Stored(reader) => reader,
            Self::Zlib(reader) => reader.get_ref(),
            Self::Lzma1(reader) => reader.inner(),
        }
    }

    /// Gets a mutable reference to the underlying reader.
    ///
    /// It is inadvisable to directly read from the underlying reader.
    #[must_use]
    pub fn get_mut(&mut self) -> &mut R {
        match self {
            Self::Stored(reader) => reader,
            Self::Zlib(reader) => reader.get_mut(),
            Self::Lzma1(reader) => reader.inner_mut(),
        }
    }

    /// Consumes this decoder, returning the underlying reader.
    #[must_use]
    pub fn into_inner(self) -> R {
        match self {
            Self::Stored(reader) => reader,
            Self::Zlib(reader) => reader.into_inner(),
            Self::Lzma1(reader) => reader.into_inner(),
        }
    }
}

impl<R: io::Read> io::Read for Decoder<R> {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        match self {
            Self::Stored(reader) => reader.read(buf),
            Self::Zlib(reader) => reader.read(buf),
            Self::Lzma1(reader) => reader.read(buf),
        }
    }
}
