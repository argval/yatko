use std::array::TryFromSliceError;

use zerocopy::{FromBytes, Immutable, KnownLayout};

/// <https://github.com/jrsoftware/issrc/blob/is-6_5_1/Projects/Src/Shared.Struct.pas#L70>
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, FromBytes, Immutable, KnownLayout)]
#[repr(transparent)]
pub struct KDFSalt([u8; 16]);

impl KDFSalt {
    /// Creates a new KDF Salt from an array of 16 bytes.
    #[must_use]
    #[inline]
    pub const fn new(salt: [u8; 16]) -> Self {
        Self(salt)
    }

    /// Returns the inner KDF Salt array.
    #[must_use]
    #[inline]
    pub const fn inner(&self) -> &[u8; 16] {
        &self.0
    }

    /// Consumes the KDF Salt, returning the inner array.
    #[must_use]
    #[inline]
    pub const fn into_inner(self) -> [u8; 16] {
        self.0
    }
}

impl AsRef<[u8]> for KDFSalt {
    fn as_ref(&self) -> &[u8] {
        &self.0
    }
}

impl From<[u8; 16]> for KDFSalt {
    fn from(array: [u8; 16]) -> Self {
        Self::new(array)
    }
}

impl TryFrom<&[u8]> for KDFSalt {
    type Error = TryFromSliceError;

    fn try_from(slice: &[u8]) -> Result<Self, Self::Error> {
        <[u8; 16]>::try_from(slice).map(Self::new)
    }
}
