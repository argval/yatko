use std::{fmt, io};

use encoding_rs::{Encoding, WINDOWS_1252};
use zerocopy::LittleEndian;

use crate::read::ReadBytesExt;

/// A UCSD Pascal-style string.
#[derive(Clone, Eq, PartialEq)]
pub enum PascalString {
    Utf8(String),
    Bytes(Vec<u8>),
}

impl PascalString {
    /// <https://github.com/jrsoftware/issrc/blob/is-6_4_3/Projects/Src/Setup.SpawnClient.pas#L45>
    pub fn read<R>(mut reader: R) -> io::Result<Option<Self>>
    where
        R: io::Read,
    {
        let length = reader.read_u32::<LittleEndian>()?;

        Self::read_sized(reader, length)
    }

    pub fn read_sized<R>(mut reader: R, size: u32) -> io::Result<Option<Self>>
    where
        R: io::Read,
    {
        if size == 0 {
            return Ok(None);
        }

        let mut buffer = vec![0; size as usize];
        reader.read_exact(&mut buffer)?;

        Ok(Some(Self::Bytes(buffer)))
    }

    pub fn read_decoded<R>(reader: R, codepage: &'static Encoding) -> io::Result<Option<Self>>
    where
        R: io::Read,
    {
        Ok(Self::read(reader)?.map(|str| str.decoded(codepage)))
    }

    pub fn read_sized_decoded<R>(
        reader: R,
        size: u32,
        codepage: &'static Encoding,
    ) -> io::Result<Option<Self>>
    where
        R: io::Read,
    {
        Ok(Self::read_sized(reader, size)?.map(|str| str.decoded(codepage)))
    }

    /// Decodes the string using the specified codepage.
    pub fn decode(&mut self, codepage: &'static Encoding) {
        match self {
            Self::Utf8(_) => {}
            Self::Bytes(bytes) => {
                *self = Self::Utf8(
                    codepage
                        .decode_without_bom_handling_and_without_replacement(bytes)
                        .unwrap_or_default()
                        .into_owned(),
                );
            }
        }
    }

    /// Decodes the string using the specified codepage and returns it.
    #[must_use]
    pub fn decoded(mut self, codepage: &'static Encoding) -> Self {
        self.decode(codepage);
        self
    }

    /// Returns `true` if the string has been decoded to UTF-8, otherwise returns `false`.
    #[must_use]
    #[inline]
    pub const fn is_decoded(&self) -> bool {
        matches!(self, Self::Utf8(_))
    }

    /// Returns the string as a `String` if it has been decoded, attempting to decode it as UTF-8 if
    /// it has not.
    #[must_use]
    pub fn into_string(self) -> String {
        match self {
            Self::Utf8(str) => str,
            Self::Bytes(bytes) => String::from_utf8_lossy(&bytes).into_owned(),
        }
    }

    /// Returns the string as a string slice if it has been decoded, attempting to decode it as
    /// UTF-8 if it has not.
    #[must_use]
    pub fn as_str(&self) -> &str {
        match self {
            Self::Utf8(str) => str.as_str(),
            Self::Bytes(bytes) => std::str::from_utf8(bytes).unwrap_or_default(),
        }
    }
}

impl Default for PascalString {
    fn default() -> Self {
        Self::Utf8(String::default())
    }
}

impl fmt::Debug for PascalString {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Utf8(str) => str.fmt(f),
            Self::Bytes(bytes) => write!(
                f,
                "{:?}",
                WINDOWS_1252
                    .decode_without_bom_handling_and_without_replacement(bytes)
                    .unwrap_or_default()
            ),
        }
    }
}

impl fmt::Display for PascalString {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Utf8(str) => str.fmt(f),
            Self::Bytes(bytes) => {
                let decoded = WINDOWS_1252
                    .decode_without_bom_handling_and_without_replacement(bytes)
                    .unwrap_or_default();
                decoded.fmt(f)
            }
        }
    }
}

impl From<&str> for PascalString {
    fn from(s: &str) -> Self {
        Self::Utf8(s.to_owned())
    }
}
