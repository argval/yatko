mod encryption_use;
mod kdf_salt;
mod nonce;
mod reader;

use std::io::Read;

pub use encryption_use::EncryptionUse;
pub use kdf_salt::KDFSalt;
pub use nonce::EncryptionNonce;
use zerocopy::LE;

use crate::{
    encryption::reader::EncryptionHeaderReader,
    entry::checksum::ChecksumMismatchError,
    error::{InnoError, InnoResult},
    read::{ReadBytesExt, crc32::Crc32Reader},
    version::InnoVersion,
};

/// <https://github.com/jrsoftware/issrc/blob/is-6_7_3/Projects/Src/Shared.Struct.pas#L105>
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
#[repr(C)]
pub struct EncryptionHeader {
    encryption_use: EncryptionUse,
    kdf_salt: KDFSalt,
    kdf_iterations: u32,
    base_nonce: EncryptionNonce,
    password_test: u32,
}

impl EncryptionHeader {
    pub fn read<R>(mut reader: R, version: InnoVersion) -> InnoResult<Self>
    where
        R: Read,
    {
        let crc32 = if version >= 6.5 {
            reader.read_u32::<LE>()?
        } else {
            0
        };

        let mut reader = if version >= 6.5 {
            EncryptionHeaderReader::CRC32(Crc32Reader::new(&mut reader))
        } else {
            EncryptionHeaderReader::None(&mut reader)
        };

        let mut encryption_header = Self {
            encryption_use: if version >= 6.5 {
                EncryptionUse::try_read_from_io(&mut reader)?
            } else {
                EncryptionUse::None
            },
            password_test: if version < 6.5 {
                reader.read_u32::<LE>()?
            } else {
                0
            },
            kdf_salt: reader.read_t::<KDFSalt>()?,
            kdf_iterations: reader.read_u32::<LE>()?,
            base_nonce: reader.read_t::<EncryptionNonce>()?,
        };

        if version >= 6.5 {
            encryption_header.password_test = reader.read_u32::<LE>()?;
        }

        // Check if the expected CRC-32 matches the calculated CRC-32.
        let actual_crc32 = reader.finalize();
        if crc32 != actual_crc32 {
            return Err(InnoError::ChecksumMismatch {
                location: "Encryption header",
                inner: ChecksumMismatchError::new_adler32(crc32, actual_crc32),
            });
        }

        Ok(encryption_header)
    }

    #[must_use]
    #[inline]
    pub const fn encryption_use(&self) -> EncryptionUse {
        self.encryption_use
    }

    #[must_use]
    #[inline]
    pub const fn kdf_salt(&self) -> KDFSalt {
        self.kdf_salt
    }

    #[must_use]
    #[inline]
    pub const fn kdf_iterations(&self) -> u32 {
        self.kdf_iterations
    }

    #[must_use]
    #[inline]
    pub const fn base_nonce(&self) -> EncryptionNonce {
        self.base_nonce
    }

    #[must_use]
    #[inline]
    pub const fn password_test(&self) -> u32 {
        self.password_test
    }
}
