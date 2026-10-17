use std::io;

use zerocopy::{Immutable, KnownLayout, TryFromBytes, ValidityError, try_transmute};

use super::HeaderFlags;

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum AutoBool {
    #[default]
    Auto = 0,
    No = 1,
    Yes = 2,
}

impl AutoBool {
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
    pub const fn from_header_flags(flags: &HeaderFlags, flag: HeaderFlags) -> Self {
        if flags.contains(flag) {
            Self::Yes
        } else {
            Self::No
        }
    }

    #[must_use]
    pub const fn as_str(&self) -> &'static str {
        match self {
            Self::Auto => "Auto",
            Self::No => "No",
            Self::Yes => "Yes",
        }
    }
}

impl From<bool> for AutoBool {
    fn from(value: bool) -> Self {
        if value { Self::Yes } else { Self::No }
    }
}

impl TryFrom<u8> for AutoBool {
    type Error = ValidityError<u8, Self>;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        try_transmute!(value)
    }
}
