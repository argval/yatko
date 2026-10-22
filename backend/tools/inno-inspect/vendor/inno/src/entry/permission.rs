use std::{fmt, io};

use crate::{read::ReadBytesExt, string::PascalString};

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct Permission(PascalString);

impl Permission {
    pub fn read<R>(read: &mut R) -> io::Result<Self>
    where
        R: io::Read,
    {
        read.read_pascal_string()
            .map(|str| Self(str.unwrap_or_default()))
    }

    /// Extracts a string slice containing the entire `Permission`.
    #[must_use]
    #[inline]
    pub fn as_str(&self) -> &str {
        self.0.as_str()
    }
}

impl AsRef<str> for Permission {
    #[inline]
    fn as_ref(&self) -> &str {
        self.as_str()
    }
}

impl fmt::Display for Permission {
    #[inline]
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.0.fmt(f)
    }
}
