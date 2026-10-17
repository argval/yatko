use std::io;

use crate::read::crc32::Crc32Reader;

pub enum EncryptionHeaderReader<R: io::Read> {
    CRC32(Crc32Reader<R>),
    None(R),
}

impl<R: io::Read> EncryptionHeaderReader<R> {
    /// Gets a reference to the underlying reader.
    pub const fn get_ref(&self) -> &R {
        match self {
            Self::CRC32(reader) => reader.get_ref(),
            Self::None(reader) => reader,
        }
    }

    /// Provides mutable access to the inner reader without affecting the hasher.
    pub const fn get_mut(&mut self) -> &mut R {
        match self {
            Self::CRC32(reader) => reader.get_mut(),
            Self::None(reader) => reader,
        }
    }

    /// Finalize the reader and return the computed CRC32 value, if any.
    pub fn finalize(self) -> u32 {
        match self {
            Self::CRC32(reader) => reader.finalize(),
            Self::None(_) => 0,
        }
    }
}

impl<R: io::Read> io::Read for EncryptionHeaderReader<R> {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        match self {
            Self::CRC32(reader) => reader.read(buf),
            Self::None(reader) => reader.read(buf),
        }
    }
}
