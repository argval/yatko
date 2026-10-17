use core::fmt;
use std::array::TryFromSliceError;

use zerocopy::{FromBytes, Immutable, KnownLayout};

#[derive(Clone, Copy, Eq, PartialEq, FromBytes, Immutable, KnownLayout)]
#[repr(transparent)]
pub struct Sha1([u8; 20]);

impl Sha1 {
    /// Creates a new SHA-1 from an array of 20 bytes.
    #[must_use]
    #[inline]
    pub const fn new(sha1: [u8; 20]) -> Self {
        Self(sha1)
    }

    /// Returns the inner SHA-1 array.
    #[must_use]
    #[inline]
    pub const fn inner(&self) -> &[u8; 20] {
        &self.0
    }

    /// Consumes the SHA-1, returning the inner array.
    #[must_use]
    #[inline]
    pub const fn into_inner(self) -> [u8; 20] {
        self.0
    }
}

impl AsRef<[u8]> for Sha1 {
    fn as_ref(&self) -> &[u8] {
        &self.0
    }
}

impl fmt::Debug for Sha1 {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{self}")
    }
}

impl fmt::Display for Sha1 {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        for byte in self.0 {
            write!(f, "{byte:02X}")?;
        }
        Ok(())
    }
}

impl From<[u8; 20]> for Sha1 {
    fn from(array: [u8; 20]) -> Self {
        Self::new(array)
    }
}

impl TryFrom<&[u8]> for Sha1 {
    type Error = TryFromSliceError;

    fn try_from(slice: &[u8]) -> Result<Self, Self::Error> {
        <[u8; 20]>::try_from(slice).map(Self::new)
    }
}

impl PartialEq<[u8; 20]> for Sha1 {
    fn eq(&self, other: &[u8; 20]) -> bool {
        self.inner() == other
    }
}

impl PartialEq<Sha1> for [u8; 20] {
    fn eq(&self, other: &Sha1) -> bool {
        self == other.inner()
    }
}

#[cfg(test)]
mod tests {
    use super::Sha1;

    #[test]
    fn size() {
        assert_eq!(size_of::<Sha1>(), 20);
    }
}
