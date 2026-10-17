use std::{fmt, fmt::Write, io};

use super::InnoVersion;
use crate::version::VersionVariant;

const SIGNATURE_LEN: usize = 12;

#[derive(Clone, Copy, Eq, PartialEq)]
pub struct SetupLoaderSignature([u8; SIGNATURE_LEN]);

impl SetupLoaderSignature {
    /// The length of the signature in bytes.
    pub const LEN: usize = SIGNATURE_LEN;

    const KNOWN: [(Self, InnoVersion); 7] = [
        (Self(*b"rDlPtS02\x87eVx"), InnoVersion::new(1, 2, 10, 0)),
        (Self(*b"rDlPtS04\x87eVx"), InnoVersion::new(4, 0, 0, 0)),
        (Self(*b"rDlPtS05\x87eVx"), InnoVersion::new(4, 0, 3, 0)),
        (Self(*b"rDlPtS06\x87eVx"), InnoVersion::new(4, 0, 10, 0)),
        (Self(*b"rDlPtS07\x87eVx"), InnoVersion::new(4, 1, 6, 0)),
        (
            Self(*b"rDlPtS\xCD\xE6\xD7{\x0B*"),
            InnoVersion::new(5, 1, 5, 0),
        ),
        (
            Self(*b"nS5W7dT\x83\xAA\x1B\x0Fj"),
            InnoVersion::new(5, 1, 5, 0),
        ),
    ];

    const KNOWN_LEGACY: [(Self, InnoVersion); 2] = [
        (
            Self(*b"i1.2.10--16\x1A"),
            InnoVersion::new_with_variant(1, 2, 10, 0, VersionVariant::BITS_16),
        ),
        (Self(*b"i1.2.10--32\x1A"), InnoVersion::new(1, 2, 10, 0)),
    ];

    /// Reads a `SetupLoaderSignature` from a reader.
    pub fn read_from<R>(mut src: R) -> io::Result<Self>
    where
        R: io::Read,
    {
        let mut signature = [0; Self::LEN];
        src.read_exact(&mut signature)?;
        Ok(Self::new(signature))
    }

    /// Creates a new `SetupLoaderSignature` from a byte array.
    #[must_use]
    #[inline]
    pub const fn new(signature: [u8; Self::LEN]) -> Self {
        Self(signature)
    }

    /// Returns `true` if the signature is known.
    #[must_use]
    pub fn is_known(self) -> bool {
        Self::KNOWN.into_iter().any(|(sig, _)| sig == self)
    }

    /// Returns the number of bits of the Inno Setup version the signature represents.
    #[must_use]
    pub fn bits(self) -> u32 {
        if self.0[0] == b'i'
            && self.0[11] == b'\x1A'
            && let Ok(Ok(bits)) = std::str::from_utf8(&self.0[9..=10]).map(str::parse::<u32>)
        {
            bits
        } else {
            u32::BITS
        }
    }

    /// Returns `true` if the signature is 16-bit.
    #[must_use]
    pub fn is_16_bit(self) -> bool {
        self.bits() == u16::BITS
    }

    /// Returns the signature as a byte array.
    #[must_use]
    #[inline]
    pub const fn as_array(self) -> [u8; Self::LEN] {
        self.0
    }

    /// Returns the version associated with the signature, if known.
    #[must_use]
    pub fn version(self) -> Option<InnoVersion> {
        Self::KNOWN
            .into_iter()
            .find_map(|(sig, version)| (sig == self).then_some(version))
    }
}

impl AsRef<[u8; SIGNATURE_LEN]> for SetupLoaderSignature {
    fn as_ref(&self) -> &[u8; SIGNATURE_LEN] {
        &self.0
    }
}

impl From<[u8; SIGNATURE_LEN]> for SetupLoaderSignature {
    fn from(signature: [u8; SIGNATURE_LEN]) -> Self {
        Self::new(signature)
    }
}

impl fmt::Debug for SetupLoaderSignature {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, r#""{self}""#)
    }
}

impl fmt::Display for SetupLoaderSignature {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_array()
            .into_iter()
            .flat_map(std::ascii::escape_default)
            .map(char::from)
            .try_for_each(|escaped| f.write_char(escaped))
    }
}
