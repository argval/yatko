use core::fmt;

use md5::Digest;

use super::{Checksum, ChecksumMismatchError, Md5, Sha1, Sha256};

/// A [`Checksum`] being computed as bytes arrive, for a reader that never
/// holds the whole of what it is checking.
#[derive(Debug)]
pub(crate) struct ChecksumHasher(State);

enum State {
    Adler32 {
        expected: u32,
        state: simd_adler32::Adler32,
    },
    Crc32 {
        expected: u32,
        state: crc32fast::Hasher,
    },
    Md5 {
        expected: [u8; 16],
        state: ::md5::Md5,
    },
    Sha1 {
        expected: [u8; 20],
        state: ::sha1::Sha1,
    },
    Sha256 {
        expected: [u8; 32],
        state: ::sha2::Sha256,
    },
    Unchecked,
}

/// By hand: `simd_adler32::Adler32` is not [`Debug`], and the hasher states are
/// opaque anyway.
impl fmt::Debug for State {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Adler32 { expected, .. } => f.debug_tuple("Adler32").field(expected).finish(),
            Self::Crc32 { expected, .. } => f.debug_tuple("Crc32").field(expected).finish(),
            Self::Md5 { expected, .. } => f.debug_tuple("Md5").field(&Md5::new(*expected)).finish(),
            Self::Sha1 { expected, .. } => {
                f.debug_tuple("Sha1").field(&Sha1::new(*expected)).finish()
            }
            Self::Sha256 { expected, .. } => f
                .debug_tuple("Sha256")
                .field(&Sha256::new(*expected))
                .finish(),
            Self::Unchecked => f.write_str("Unchecked"),
        }
    }
}

impl ChecksumHasher {
    pub fn update(&mut self, data: &[u8]) {
        match &mut self.0 {
            State::Adler32 { state, .. } => state.write(data),
            State::Crc32 { state, .. } => state.update(data),
            State::Md5 { state, .. } => state.update(data),
            State::Sha1 { state, .. } => state.update(data),
            State::Sha256 { state, .. } => state.update(data),
            State::Unchecked => {}
        }
    }

    /// # Errors
    ///
    /// Returns [`ChecksumMismatchError`] if what was written does not match
    /// what the installer recorded.
    pub fn finish(self) -> Result<(), ChecksumMismatchError> {
        match self.0 {
            State::Adler32 { expected, state } => {
                let actual = state.finish();
                if expected != actual {
                    return Err(ChecksumMismatchError::new_adler32(expected, actual));
                }
            }
            State::Crc32 { expected, state } => {
                let actual = state.finalize();
                if expected != actual {
                    return Err(ChecksumMismatchError::new_crc32(expected, actual));
                }
            }
            State::Md5 { expected, state } => {
                let actual = state.finalize().0;
                if expected != actual {
                    return Err(ChecksumMismatchError::new_md5(expected, actual));
                }
            }
            State::Sha1 { expected, state } => {
                let actual = state.finalize().0;
                if expected != actual {
                    return Err(ChecksumMismatchError::new_sha1(expected, actual));
                }
            }
            State::Sha256 { expected, state } => {
                let actual = state.finalize().0;
                if expected != actual {
                    return Err(ChecksumMismatchError::new_sha256(expected, actual));
                }
            }
            State::Unchecked => {}
        }
        Ok(())
    }
}

impl Checksum {
    /// Starts computing this checksum incrementally.
    #[must_use]
    pub(crate) fn hasher(self) -> ChecksumHasher {
        ChecksumHasher(match self {
            Self::Adler32(expected) => State::Adler32 {
                expected,
                state: simd_adler32::Adler32::new(),
            },
            Self::Crc32(expected) => State::Crc32 {
                expected,
                state: crc32fast::Hasher::new(),
            },
            Self::MD5(expected) => State::Md5 {
                expected: expected.into_inner(),
                state: ::md5::Md5::new(),
            },
            Self::Sha1(expected) => State::Sha1 {
                expected: expected.into_inner(),
                state: ::sha1::Sha1::new(),
            },
            Self::Sha256(expected) => State::Sha256 {
                expected: expected.into_inner(),
                state: ::sha2::Sha256::new(),
            },
            Self::Check(_) => State::Unchecked,
        })
    }
}

#[cfg(test)]
mod tests {
    use md5::Digest;

    use super::super::Checksum;

    const DATA: &[u8] = b"the quick brown fox jumps over the lazy dog";

    /// Feeding a checksum in pieces must agree with feeding it whole, or a
    /// streaming read would reject sound data.
    #[test]
    fn every_variant_agrees_with_the_whole_buffer_form() {
        let (first, second) = DATA.split_at(10);

        for checksum in [
            Checksum::new_adler32(simd_adler32::adler32(&DATA)),
            Checksum::new_crc32(crc32fast::hash(DATA)),
            Checksum::new_md5(<[u8; 16]>::from(::md5::Md5::digest(DATA))),
            Checksum::new_sha1(<[u8; 20]>::from(::sha1::Sha1::digest(DATA))),
            Checksum::new_sha256(<[u8; 32]>::from(::sha2::Sha256::digest(DATA))),
        ] {
            checksum.validate(DATA).expect("whole buffer");

            let mut hasher = checksum.hasher();
            hasher.update(first);
            hasher.update(second);
            hasher.finish().expect("incremental");
        }
    }

    #[test]
    fn a_mismatch_is_reported() {
        let checksum = Checksum::new_crc32(crc32fast::hash(DATA));
        let mut hasher = checksum.hasher();
        hasher.update(b"something else entirely");
        assert!(hasher.finish().is_err());
    }

    /// The legacy `Check` variant is not verified, here or in `validate`.
    #[test]
    fn the_legacy_check_variant_accepts_anything() {
        let mut hasher = Checksum::Check([1, 2, 3, 4]).hasher();
        hasher.update(b"whatever");
        assert!(hasher.finish().is_ok());
    }

    #[test]
    fn nothing_written_still_finishes() {
        let checksum = Checksum::new_crc32(crc32fast::hash(b""));
        assert!(checksum.hasher().finish().is_ok());
    }
}
