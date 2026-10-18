use core::fmt;
use std::array::TryFromSliceError;

use zerocopy::{FromBytes, Immutable, KnownLayout};

/// The length of a SHA-256 hash in bytes.
const SHA256_LEN: usize = 256 / u8::BITS as usize;

#[derive(Clone, Copy, Eq, PartialEq, FromBytes, Immutable, KnownLayout)]
#[repr(transparent)]
pub struct Sha256([u8; SHA256_LEN]);

impl Sha256 {
    /// Creates a new SHA-256 from an array of 32 bytes.
    #[must_use]
    #[inline]
    pub const fn new(sha256: [u8; SHA256_LEN]) -> Self {
        Self(sha256)
    }

    /// Returns the inner SHA-256 array.
    #[must_use]
    #[inline]
    pub const fn inner(&self) -> &[u8; SHA256_LEN] {
        &self.0
    }

    /// Consumes the SHA-256, returning the inner array.
    #[must_use]
    #[inline]
    pub const fn into_inner(self) -> [u8; SHA256_LEN] {
        self.0
    }
}

impl AsRef<[u8]> for Sha256 {
    fn as_ref(&self) -> &[u8] {
        &self.0
    }
}

impl fmt::Debug for Sha256 {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{self}")
    }
}

impl fmt::Display for Sha256 {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        for byte in self.0 {
            write!(f, "{byte:02X}")?;
        }
        Ok(())
    }
}

impl From<[u8; SHA256_LEN]> for Sha256 {
    fn from(array: [u8; SHA256_LEN]) -> Self {
        Self::new(array)
    }
}

impl TryFrom<&[u8]> for Sha256 {
    type Error = TryFromSliceError;

    fn try_from(slice: &[u8]) -> Result<Self, Self::Error> {
        <[u8; SHA256_LEN]>::try_from(slice).map(Self::new)
    }
}

impl PartialEq<[u8; SHA256_LEN]> for Sha256 {
    fn eq(&self, other: &[u8; SHA256_LEN]) -> bool {
        self.inner() == other
    }
}

impl PartialEq<Sha256> for [u8; SHA256_LEN] {
    fn eq(&self, other: &Sha256) -> bool {
        self == other.inner()
    }
}

#[cfg(test)]
mod tests {
    use super::{SHA256_LEN, Sha256};

    #[test]
    fn size() {
        assert_eq!(size_of::<Sha256>(), SHA256_LEN);
    }
}
