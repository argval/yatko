use std::{fmt, io};

use zerocopy::{Immutable, KnownLayout, TryFromBytes, ValidityError, try_transmute};

/// <https://github.com/jrsoftware/issrc/blob/is-6_7_3/Projects/Src/Shared.Struct.pas#L98>
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum LightControlStyling {
    #[default]
    All = 0,
    AllButButtons = 1,
    OnlyRequired = 2,
}

impl LightControlStyling {
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
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::All => "All",
            Self::AllButButtons => "All but buttons",
            Self::OnlyRequired => "Only required",
        }
    }
}

impl fmt::Display for LightControlStyling {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}

impl TryFrom<u8> for LightControlStyling {
    type Error = ValidityError<u8, Self>;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        try_transmute!(value)
    }
}
