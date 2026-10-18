use std::fmt;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum Compression {
    Stored(u64),
    Zlib(u64),
    LZMA1(u64),
}

impl Compression {
    /// Returns the size of the compressed bytes.
    #[must_use]
    pub const fn size(self) -> u64 {
        match self {
            Self::Stored(size) | Self::Zlib(size) | Self::LZMA1(size) => size,
        }
    }

    /// Returns a mutable reference to the size of the compressed bytes.
    #[must_use]
    pub const fn size_mut(&mut self) -> &mut u64 {
        match self {
            Self::Stored(size) | Self::Zlib(size) | Self::LZMA1(size) => size,
        }
    }

    /// Returns true if the compression is stored (no compression).
    #[must_use]
    #[inline]
    pub const fn is_stored(self) -> bool {
        matches!(self, Self::Stored(_))
    }

    /// Returns true if the compression is Zlib.
    #[must_use]
    #[inline]
    pub const fn is_zlib(self) -> bool {
        matches!(self, Self::Zlib(_))
    }

    /// Returns true if the compression is LZMA1.
    #[must_use]
    #[inline]
    pub const fn is_lzma1(self) -> bool {
        matches!(self, Self::LZMA1(_))
    }
}

impl fmt::Display for Compression {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Stored(_) => f.write_str("Stored"),
            Self::Zlib(_) => f.write_str("Zlib"),
            Self::LZMA1(_) => f.write_str("LZMA1"),
        }
    }
}
