mod encryption;

pub use encryption::Encryption;

use crate::header::Compression;

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct Chunk {
    pub(crate) first_slice: u32,
    pub(crate) last_slice: u32,
    pub(crate) start_offset: u64,
    pub(crate) sub_offset: u64,
    pub(crate) original_size: u64,
    pub(crate) compression: Compression,
    pub(crate) encryption: Encryption,
}

impl Chunk {
    /// Returns the first disk slice containing this chunk.
    #[must_use]
    #[inline]
    pub const fn first_slice(&self) -> u32 {
        self.first_slice
    }

    /// Returns the last disk slice containing this chunk.
    #[must_use]
    #[inline]
    pub const fn last_slice(&self) -> u32 {
        self.last_slice
    }

    /// Returns the start offset of the chunk within the data stream.
    #[must_use]
    #[inline]
    pub const fn start_offset(&self) -> u64 {
        self.start_offset
    }

    /// Returns the sub-offset within the data stream.
    #[must_use]
    #[inline]
    pub const fn sub_offset(&self) -> u64 {
        self.sub_offset
    }

    /// Returns the original (compressed) size of the chunk.
    #[must_use]
    #[inline]
    pub const fn original_size(&self) -> u64 {
        self.original_size
    }

    /// Returns the compression method used for this chunk.
    #[must_use]
    #[inline]
    pub const fn compression(&self) -> Compression {
        self.compression
    }

    /// Returns the encryption method used for this chunk.
    #[must_use]
    #[inline]
    pub const fn encryption(&self) -> Encryption {
        self.encryption
    }

    /// Returns `true` if the chunk is compressed.
    #[must_use]
    #[inline]
    pub const fn is_compressed(&self) -> bool {
        !self.compression().is_stored()
    }

    /// Returns `true` if this chunk is encrypted.
    #[must_use]
    #[inline]
    pub const fn is_encrypted(self) -> bool {
        !self.encryption().is_plaintext()
    }
}
