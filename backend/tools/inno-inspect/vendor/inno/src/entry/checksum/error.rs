use thiserror::Error;

use super::{Checksum, Md5, Sha1, Sha256};

#[derive(Error, Debug)]
pub enum ChecksumMismatchError {
    #[error("Adler32 checksum mismatch. Expected {expected} but calculated {actual}")]
    Adler32 { expected: u32, actual: u32 },
    #[error("CRC-32 checksum mismatch. Expected {expected} but calculated {actual}")]
    Crc32 { expected: u32, actual: u32 },
    #[error("MD5 checksum mismatch. Expected {expected} but calculated {actual}")]
    Md5 { expected: Md5, actual: Md5 },
    #[error("SHA-1 checksum mismatch. Expected {expected} but calculated {actual}")]
    Sha1 { expected: Sha1, actual: Sha1 },
    #[error("SHA-256 checksum mismatch. Expected {expected} but calculated {actual}")]
    Sha256 { expected: Sha256, actual: Sha256 },
}

impl ChecksumMismatchError {
    /// Creates a new Adler32 checksum mismatch error.
    #[must_use]
    #[inline]
    pub(crate) const fn new_adler32(expected: u32, actual: u32) -> Self {
        Self::Adler32 { expected, actual }
    }

    /// Creates a new CRC-32 checksum mismatch error.
    #[must_use]
    #[inline]
    pub(crate) const fn new_crc32(expected: u32, actual: u32) -> Self {
        Self::Crc32 { expected, actual }
    }

    /// Creates a new MD5 checksum mismatch error.
    #[must_use]
    #[inline]
    pub(crate) const fn new_md5(expected: [u8; 16], actual: [u8; 16]) -> Self {
        Self::Md5 {
            expected: Md5::new(expected),
            actual: Md5::new(actual),
        }
    }

    /// Creates a new SHA-1 checksum mismatch error.
    #[must_use]
    #[inline]
    pub(crate) const fn new_sha1(expected: [u8; 20], actual: [u8; 20]) -> Self {
        Self::Sha1 {
            expected: Sha1::new(expected),
            actual: Sha1::new(actual),
        }
    }

    /// Creates a new SHA-256 checksum mismatch error.
    #[must_use]
    #[inline]
    pub(crate) const fn new_sha256(expected: [u8; 32], actual: [u8; 32]) -> Self {
        Self::Sha256 {
            expected: Sha256::new(expected),
            actual: Sha256::new(actual),
        }
    }

    /// Returns the expected checksum.
    #[must_use]
    pub const fn expected(&self) -> Checksum {
        match self {
            Self::Adler32 { expected, .. } => Checksum::Adler32(*expected),
            Self::Crc32 { expected, .. } => Checksum::Crc32(*expected),
            Self::Md5 { expected, .. } => Checksum::MD5(*expected),
            Self::Sha1 { expected, .. } => Checksum::Sha1(*expected),
            Self::Sha256 { expected, .. } => Checksum::Sha256(*expected),
        }
    }

    /// Returns the actual checksum.
    #[must_use]
    pub const fn actual(&self) -> Checksum {
        match self {
            Self::Adler32 { actual, .. } => Checksum::Adler32(*actual),
            Self::Crc32 { actual, .. } => Checksum::Crc32(*actual),
            Self::Md5 { actual, .. } => Checksum::MD5(*actual),
            Self::Sha1 { actual, .. } => Checksum::Sha1(*actual),
            Self::Sha256 { actual, .. } => Checksum::Sha256(*actual),
        }
    }
}
