use std::{fmt, io};

use zerocopy::{Immutable, KnownLayout, TryFromBytes, ValidityError, try_transmute};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum LogMode {
    Append = 0,
    #[default]
    New = 1,
    Overwrite = 2,
}

impl LogMode {
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

    #[must_use]
    pub const fn as_str(&self) -> &'static str {
        match self {
            Self::Append => "Append",
            Self::New => "New",
            Self::Overwrite => "Overwrite",
        }
    }
}

impl fmt::Display for LogMode {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}

impl TryFrom<u8> for LogMode {
    type Error = ValidityError<u8, Self>;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        try_transmute!(value)
    }
}
