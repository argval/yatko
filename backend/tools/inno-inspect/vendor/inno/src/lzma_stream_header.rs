use zerocopy::{FromBytes, Immutable, KnownLayout, LE, U32};

/// The 5-byte raw LZMA1 properties header used by Inno Setup's LZMA1 streams (same layout as the
/// `.lzma` format): a single properties byte encoding lc/lp/pb, followed by a little-endian `u32`
/// dictionary size.
#[derive(Clone, Copy, Debug, Eq, PartialEq, FromBytes, Immutable, KnownLayout)]
#[repr(C)]
pub struct LzmaStreamHeader {
    props: u8,
    dict_size: U32<LE>,
}

impl LzmaStreamHeader {
    /// Returns the LZMA properties byte.
    #[must_use]
    #[inline]
    pub const fn props(self) -> u8 {
        self.props
    }

    /// Returns the LZMA dictionary size.
    #[must_use]
    #[inline]
    pub const fn dictionary_size(self) -> u32 {
        self.dict_size.get()
    }
}
