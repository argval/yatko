use std::{fmt, io};

use zerocopy::{Immutable, KnownLayout, TryFromBytes};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum TargetType {
    #[default]
    File = 0,
    FilesAndSubDirectories = 1,
    DirectoryIfEmpty = 2,
}

impl TargetType {
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

    /// Returns the target type as a static string.
    #[must_use]
    pub const fn as_str(&self) -> &'static str {
        match self {
            Self::File => "File",
            Self::FilesAndSubDirectories => "Files and subdirectories",
            Self::DirectoryIfEmpty => "Directory if empty",
        }
    }
}

impl fmt::Display for TargetType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}
