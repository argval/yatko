use std::io;

use zerocopy::{Immutable, KnownLayout, TryFromBytes, ValidityError, try_transmute};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum WaitCondition {
    #[default]
    WaitUntilTerminated = 0,
    NoWait = 1,
    WaitUntilIdle = 2,
}

impl WaitCondition {
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

impl TryFrom<u8> for WaitCondition {
    type Error = ValidityError<u8, Self>;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        try_transmute!(value)
    }
}
