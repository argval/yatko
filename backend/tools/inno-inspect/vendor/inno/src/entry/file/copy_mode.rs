use std::io;

use zerocopy::{Immutable, KnownLayout, TryFromBytes};

use super::FileFlags;

/// The mode of copying files during installation.
///
/// Deprecated since Inno Setup 3.0.5 and replaced by [`FileFlags`].
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum FileCopyMode {
    /// Prompt the user if the file already exists and is older than the source file.
    ///
    /// This is the default behavior and is equivalent to `FileFlags::PROMPT_IF_OLDER`.
    #[default]
    Normal = 0,

    /// Only copy the file if it doesn't already exist, prompting if the existing file is older.
    ///
    /// This is equivalent to `FileFlags::ONLY_IF_DOESNT_EXIST | FileFlags::PROMPT_IF_OLDER`.
    IfDoesntExist = 1,

    /// Always overwrite the file, ignoring its version and prompting if it is older.
    ///
    /// This is equivalent to `FileFlags::IGNORE_VERSION | FileFlags::PROMPT_IF_OLDER`.
    AlwaysOverwrite = 2,

    /// Always skip copying the file if it is the same or older than the existing one.
    ///
    /// This is equivalent to `FileFlags::empty()`.
    AlwaysSkipIfSameOrOlder = 3,
}

impl FileCopyMode {
    /// Reads a copy of `FileCopyMode` from an `io::Read`.
    pub fn try_read_from_io<R>(mut src: R) -> io::Result<Self>
    where
        Self: TryFromBytes + Sized,
        R: io::Read,
    {
        let mut buf = [0; size_of::<Self>()];
        src.read_exact(&mut buf)?;
        Self::try_read_from_bytes(&buf)
            .map_err(|err| io::Error::new(io::ErrorKind::InvalidData, err.to_string()))
    }
}

impl From<FileCopyMode> for FileFlags {
    /// Converts a `FileCopyMode` into a `FileFlags`.
    fn from(mode: FileCopyMode) -> Self {
        match mode {
            FileCopyMode::Normal => Self::PROMPT_IF_OLDER,
            FileCopyMode::IfDoesntExist => Self::ONLY_IF_DOESNT_EXIST | Self::PROMPT_IF_OLDER,
            FileCopyMode::AlwaysOverwrite => Self::IGNORE_VERSION | Self::PROMPT_IF_OLDER,
            FileCopyMode::AlwaysSkipIfSameOrOlder => Self::empty(),
        }
    }
}
