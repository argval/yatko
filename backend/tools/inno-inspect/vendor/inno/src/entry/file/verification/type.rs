use std::{fmt, io};

use zerocopy::{Immutable, KnownLayout, TryFromBytes, ValidityError, try_transmute};

/// <https://github.com/jrsoftware/issrc/blob/is-6_5_1/Projects/Src/Shared.Struct.pas#L240>
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum FileVerificationType {
    #[default]
    None = 0,
    Hash = 1,
    ISSig = 2,
}

impl FileVerificationType {
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
            Self::None => "None",
            Self::Hash => "Hash",
            Self::ISSig => "IS Signature",
        }
    }
}

impl fmt::Display for FileVerificationType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}

impl TryFrom<u8> for FileVerificationType {
    type Error = ValidityError<u8, Self>;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        try_transmute!(value)
    }
}

#[cfg(test)]
mod tests {
    use std::io::Cursor;

    use super::FileVerificationType;

    #[test]
    fn read_none() {
        let buf = [0, 171, 248, 115]; // First byte is 0 for 'No verification'
        let reader = Cursor::new(buf);
        assert_eq!(
            FileVerificationType::try_read_from_io(reader).unwrap(),
            FileVerificationType::None
        );
    }
}
