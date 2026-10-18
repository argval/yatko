use super::{Checksum, CompressionFilter};
#[cfg(feature = "extract")]
use crate::entry::checksum::ChecksumMismatchError;

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct File {
    /// Offset of the file within the decompressed chunk.
    pub(crate) offset: u64,

    /// Pre-filter size of the file in the decompressed chunk.
    pub(crate) size: u64,

    /// Checksum for the file.
    pub(crate) checksum: Checksum,

    /// Additional filter used before compression.
    pub(crate) compression_filter: CompressionFilter,
}

impl File {
    /// Returns the offset of the file within the decompressed chunk.
    #[must_use]
    #[inline]
    pub const fn offset(&self) -> u64 {
        self.offset
    }

    /// Returns the pre-filter size of the file in the decompressed chunk.
    #[must_use]
    #[inline]
    pub const fn size(&self) -> u64 {
        self.size
    }

    /// Returns the checksum for the file.
    #[must_use]
    #[inline]
    pub const fn checksum(&self) -> Checksum {
        self.checksum
    }

    /// Returns the compression filter applied to the file.
    #[must_use]
    #[inline]
    pub const fn compression_filter(&self) -> CompressionFilter {
        self.compression_filter
    }

    /// Validates that the given data matches the file's checksum.
    ///
    /// # Errors
    ///
    /// Returns [`ChecksumMismatchError`] if the calculated checksum does not match the
    /// expected checksum.
    #[cfg(feature = "extract")]
    #[inline]
    pub fn validate_checksum(&self, date: &[u8]) -> Result<(), ChecksumMismatchError> {
        self.checksum().validate(date)
    }
}
