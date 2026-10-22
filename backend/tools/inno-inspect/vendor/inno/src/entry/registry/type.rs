use std::{fmt, io};

use zerocopy::{Immutable, KnownLayout, TryFromBytes};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum RegistryValueType {
    #[default]
    None = 0,
    String = 1,
    ExpandString = 2,
    DWord = 3,
    Binary = 4,
    MultiString = 5,
    QWord = 6,
}

impl RegistryValueType {
    pub fn try_read_from_io<R>(mut src: R) -> io::Result<Self>
    where
        Self: Sized,
        R: io::Read,
    {
        let mut buf = [0; size_of::<Self>()];
        src.read_exact(&mut buf)?;
        Self::try_read_from_bytes(&buf)
            .map_err(|err| io::Error::new(io::ErrorKind::InvalidData, err.to_string()))
    }

    /// Returns the registry value type as a static string.
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::None => "None",
            Self::String => "String",
            Self::ExpandString => "ExpandString",
            Self::DWord => "DWord",
            Self::Binary => "Binary",
            Self::MultiString => "MultiString",
            Self::QWord => "QWord",
        }
    }
}

impl fmt::Display for RegistryValueType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}
