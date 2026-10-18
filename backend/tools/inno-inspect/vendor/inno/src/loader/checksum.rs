use std::fmt;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum Checksum {
    Adler32(u32),
    Crc32(u32),
}

impl Checksum {
    /// Returns the checksum value as a `u32`.
    #[must_use]
    pub const fn value(self) -> u32 {
        match self {
            Self::Adler32(value) | Self::Crc32(value) => value,
        }
    }

    /// Returns `true` if the checksum is an Adler32 checksum.
    #[must_use]
    #[inline]
    pub const fn is_adler32(self) -> bool {
        matches!(self, Self::Adler32(_))
    }

    /// Returns `true` if the checksum is a CRC32 checksum.
    #[must_use]
    #[inline]
    pub const fn is_crc32(self) -> bool {
        matches!(self, Self::Crc32(_))
    }
}

impl Default for Checksum {
    fn default() -> Self {
        Self::Crc32(0)
    }
}

impl fmt::Display for Checksum {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Adler32(checksum) | Self::Crc32(checksum) => write!(f, "{checksum}"),
        }
    }
}
