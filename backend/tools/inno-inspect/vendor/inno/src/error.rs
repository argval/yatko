use std::{fmt, io};

use thiserror::Error;

use super::{InnoVersion, MAX_SUPPORTED_VERSION, entry::checksum::ChecksumMismatchError};

pub type InnoResult<T> = Result<T, InnoError>;

#[derive(Error, Debug)]
pub enum InnoError {
    #[error("File is not an Inno installer")]
    NotInnoFile,
    #[error("Unexpected data at end of {0} Inno header stream")]
    UnexpectedExtraData(HeaderStream),
    #[error(
        "Inno Setup version {0} is newer than the maximum supported version {max_version}",
        max_version = MAX_SUPPORTED_VERSION
    )]
    UnsupportedVersion(InnoVersion),
    #[error("Unknown Inno setup version: {0}")]
    UnknownVersion(String),
    #[error("Unknown Inno Setup loader signature: {0:?}")]
    UnknownLoaderSignature([u8; 12]),
    #[error(
        "Inno Setup checksum mismatch reading {location}. Expected {} but calculated {}",
        inner.expected(),
        inner.actual()
    )]
    ChecksumMismatch {
        location: &'static str,
        inner: ChecksumMismatchError,
    },
    #[error("Unsupported {0} compression")]
    UnsupportedCompression(super::header::Compression),
    #[cfg(feature = "extract")]
    #[error("Encrypted installers are not supported for extraction")]
    Encrypted,
    #[cfg(feature = "extract")]
    #[error(
        "Invalid data chunk magic: expected {magic}, got {0:02X?}",
        magic=super::read::data_chunk::ZlibID::Magic
    )]
    InvalidChunkMagic([u8; 4]),
    #[cfg(feature = "extract")]
    #[error("File location index {index} is out of bounds (max: {max})")]
    FileLocationOutOfBounds { index: u32, max: usize },
    #[error(
        "Slices per disk is {0}, outside the 1 to {max} that Inno Setup allows",
        max = crate::slice::SlicesPerDisk::MAX
    )]
    InvalidSlicesPerDisk(u32),
    #[error(transparent)]
    Io(#[from] io::Error),
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum HeaderStream {
    Primary,
    Secondary,
}

impl HeaderStream {
    /// Returns the header stream name as a static string.
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Primary => "primary",
            Self::Secondary => "secondary",
        }
    }
}

impl fmt::Display for HeaderStream {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}
