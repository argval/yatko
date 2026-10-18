use std::io;

use zerocopy::{Immutable, KnownLayout, TryFromBytes};

use crate::entry::location::FileLocationFlags;

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum SignMode {
    #[default]
    NoSetting,
    Yes,
    Once,
    Check,
}

impl SignMode {
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
            Self::NoSetting => "No setting",
            Self::Yes => "Yes",
            Self::Once => "Once",
            Self::Check => "Check",
        }
    }
}

impl From<FileLocationFlags> for SignMode {
    fn from(flags: FileLocationFlags) -> Self {
        if flags.contains(FileLocationFlags::SIGN_ONCE) {
            Self::Once
        } else if flags.contains(FileLocationFlags::SIGN) {
            Self::Yes
        } else {
            Self::NoSetting
        }
    }
}
