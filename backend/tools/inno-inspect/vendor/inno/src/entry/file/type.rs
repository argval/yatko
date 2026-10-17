use std::{fmt, io};

use zerocopy::{Immutable, KnownLayout, TryFromBytes};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum FileType {
    #[default]
    UserFile = 0,
    UninstallExe = 1,
    RegSvrExe = 2,
}

impl FileType {
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
            Self::UserFile => "User file",
            Self::UninstallExe => "Uninstall exe",
            Self::RegSvrExe => "Reg-svr exe",
        }
    }
}

impl fmt::Display for FileType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}
