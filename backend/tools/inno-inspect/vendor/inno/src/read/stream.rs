use std::io::{Error, ErrorKind, Read, Result, Take};

use zerocopy::LE;

use crate::{
    InnoResult, ReadBytesExt,
    compression::Compression,
    entry::checksum::ChecksumMismatchError,
    error::{HeaderStream, InnoError},
    lzma_stream_header::LzmaStreamHeader,
    read::{
        block::{INNO_BLOCK_SIZE, InnoBlockReader},
        crc32::Crc32Reader,
        decoder::Decoder,
    },
    version::InnoVersion,
};

pub struct InnoStreamReader<R: Read> {
    inner: Decoder<InnoBlockReader<Take<R>>>,
    compression: Compression,
    inno_version: InnoVersion,
}

impl<R: Read> InnoStreamReader<R> {
    pub fn new(mut inner: R, version: InnoVersion) -> Result<Self> {
        let compression = Self::read_header(&mut inner, version)?;

        let mut chunk_reader = InnoBlockReader::new(inner.take(compression.size()));

        Ok(Self {
            inner: match compression {
                Compression::LZMA1(_) => {
                    let header = chunk_reader.read_t::<LzmaStreamHeader>()?;
                    Decoder::new_lzma1(chunk_reader, header)?
                }
                Compression::Zlib(_) => Decoder::new_zlib(chunk_reader),
                Compression::Stored(_) => Decoder::Stored(chunk_reader),
            },
            compression,
            inno_version: version,
        })
    }

    fn read_header(reader: &mut R, version: InnoVersion) -> Result<Compression> {
        let expected_crc32 = reader.read_u32::<LE>()?;

        let mut crc32_reader = Crc32Reader::new(reader);

        let compression = if version >= (4, 0, 9) {
            let size = if version >= 6.7 {
                crc32_reader.read_u64::<LE>()?
            } else {
                crc32_reader.read_u32::<LE>()?.into()
            };
            let compressed = crc32_reader.read_u8()? != 0;

            if compressed {
                if version >= (4, 1, 6) {
                    Compression::LZMA1(size)
                } else {
                    Compression::Zlib(size)
                }
            } else {
                Compression::Stored(size)
            }
        } else {
            let compressed_size = crc32_reader.read_u32::<LE>()?;
            let uncompressed_size = crc32_reader.read_u32::<LE>()?;

            let mut compression = if compressed_size.cast_signed() == -1 {
                Compression::Stored(uncompressed_size.into())
            } else {
                Compression::Zlib(compressed_size.into())
            };

            // Add the size of a CRC32 checksum for each 4KiB sub-block
            *compression.size_mut() += compression.size().div_ceil(INNO_BLOCK_SIZE.into()) * 4;

            compression
        };

        let actual_crc32 = crc32_reader.finalize();
        if actual_crc32 != expected_crc32 {
            return Err(Error::new(
                ErrorKind::InvalidData,
                InnoError::ChecksumMismatch {
                    location: "Inno stream header",
                    inner: ChecksumMismatchError::new_crc32(expected_crc32, actual_crc32),
                },
            ));
        }

        Ok(compression)
    }

    /// Consumes the stream reader and returns a new one.
    pub fn reset(self) -> Result<Self> {
        let version = self.inno_version;
        let reader = self
            .into_inner() // Decoder<InnoBlockReader<Take<R>>>
            .into_inner() // InnoBlockReader<Take<R>>
            .into_inner() // Take<R>
            .into_inner(); // R
        Self::new(reader, version)
    }

    /// Gets a reference to the underlying reader.
    ///
    /// It is inadvisable to directly read from the underlying reader.
    #[must_use]
    #[inline]
    pub const fn get_ref(&self) -> &Decoder<InnoBlockReader<Take<R>>> {
        &self.inner
    }

    /// Gets a mutable reference to the underlying reader.
    ///
    /// It is inadvisable to directly read from the underlying reader.
    #[must_use]
    #[inline]
    pub const fn get_mut(&mut self) -> &mut Decoder<InnoBlockReader<Take<R>>> {
        &mut self.inner
    }

    /// Consumes this stream reader, returning the underlying reader.
    #[must_use]
    #[inline]
    pub fn into_inner(self) -> Decoder<InnoBlockReader<Take<R>>> {
        self.inner
    }

    /// Checks whether the reader is at the end of the stream by attempting to read one byte.
    pub fn check_end_of_stream<S: Stream>(&mut self) -> InnoResult<()> {
        // Checking whether the underlying block reader has read as many bytes
        // as specified in the stream header is not reliable as a decoder may
        // return EOF when there is still a byte left. This is the case with
        // innosetup-7.0.2-x86.exe where its last block has just one byte of
        // data after the CRC.
        //
        // Instead, we must attempt to read a single byte to determine if there
        // is actually more data.

        let mut buf = [0; 1];

        if self.read(&mut buf)? != 0 {
            return Err(InnoError::UnexpectedExtraData(S::HEADER_STREAM));
        }

        Ok(())
    }
}

impl<R: Read> Read for InnoStreamReader<R> {
    fn read(&mut self, dest: &mut [u8]) -> Result<usize> {
        self.inner.read(dest)
    }
}

/// A type-level enumeration of Inno Setup header streams.
pub trait Stream: private::Sealed {
    const HEADER_STREAM: HeaderStream;
}

/// Primary header stream.
pub struct Primary;

impl Stream for Primary {
    const HEADER_STREAM: HeaderStream = HeaderStream::Primary;
}

/// Secondary header stream.
pub struct Secondary;

impl Stream for Secondary {
    const HEADER_STREAM: HeaderStream = HeaderStream::Secondary;
}

mod private {
    pub trait Sealed {}

    impl Sealed for super::Primary {}
    impl Sealed for super::Secondary {}
}
