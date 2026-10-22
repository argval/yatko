use std::io;

use zerocopy::{Immutable, KnownLayout, TryFromBytes, ValidityError, try_transmute};

/// <https://github.com/jrsoftware/issrc/blob/is-7_0_0/Projects/Src/Shared.Struct.pas#L100>
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum Bitness {
    #[default]
    InstallDefault = 0,
    Bit32 = 1,
    Bit64 = 2,
    NativeBit = 3,
    CurrentProcessBit = 4,
}

impl Bitness {
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
}

impl TryFrom<u8> for Bitness {
    type Error = ValidityError<u8, Self>;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        try_transmute!(value)
    }
}
