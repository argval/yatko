use std::fmt;

use zerocopy::{FromBytes, Immutable, KnownLayout, LE, U32, U64};

/// <https://github.com/jrsoftware/issrc/blob/is-6_5_1/Projects/Src/Shared.Struct.pas#L72>
#[derive(Clone, Copy, Default, Eq, PartialEq, FromBytes, Immutable, KnownLayout)]
#[repr(C)]
pub struct EncryptionNonce {
    random_xor_start_offset: U64<LE>,
    random_xor_first_slice: U32<LE>,
    remaining_random: [U32<LE>; 3],
}

impl EncryptionNonce {
    /// Returns the random XOR start offset.
    #[must_use]
    #[inline]
    pub const fn random_xor_start_offset(&self) -> u64 {
        self.random_xor_start_offset.get()
    }

    /// Returns the random XOR first slice offset.
    #[must_use]
    #[inline]
    pub const fn random_xor_first_slice(&self) -> u32 {
        self.random_xor_first_slice.get()
    }

    /// Returns the remaining random.
    #[must_use]
    pub const fn remaining_random(&self) -> [u32; 3] {
        [
            self.remaining_random[0].get(),
            self.remaining_random[1].get(),
            self.remaining_random[2].get(),
        ]
    }
}

impl fmt::Debug for EncryptionNonce {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("EncryptionNonce")
            .field("RandomXorStartOffset", &self.random_xor_start_offset())
            .field("RandomXorFirstSlice", &self.random_xor_first_slice())
            .field("RemainingRandom", &self.remaining_random())
            .finish()
    }
}

#[cfg(test)]
mod tests {
    use zerocopy::FromBytes;

    use super::EncryptionNonce;

    #[test]
    fn size() {
        assert_eq!(size_of::<EncryptionNonce>(), 24);
    }

    #[test]
    fn read() {
        let bytes = [
            1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0, 5, 0, 0, 0,
        ];
        let encryption_nonce = EncryptionNonce::read_from_bytes(&bytes).unwrap();

        assert_eq!(encryption_nonce.random_xor_start_offset(), 1);
        assert_eq!(encryption_nonce.random_xor_first_slice(), 2);
        assert_eq!(encryption_nonce.remaining_random(), [3, 4, 5]);
    }
}
