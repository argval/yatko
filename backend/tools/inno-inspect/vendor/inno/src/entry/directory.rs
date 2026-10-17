use std::{fmt, io};

use bitflags::bitflags;
use encoding_rs::Encoding;
use zerocopy::LE;

use crate::{
    ReadBytesExt,
    entry::Condition,
    string::PascalString,
    version::{InnoVersion, windows_version::WindowsVersionRange},
};

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Directory {
    name: Option<String>,
    permissions: Option<PascalString>,
    attributes: u32,
    /// Index into the permission entry list
    permission: i16,
    flags: DirectoryFlags,
}

impl Directory {
    pub fn read<R>(
        mut reader: R,
        codepage: &'static Encoding,
        version: InnoVersion,
    ) -> io::Result<Self>
    where
        R: io::Read,
    {
        if version < 1.3 {
            let _uncompressed_size = reader.read_u32::<LE>()?;
        }

        let mut directory = Self {
            name: reader.read_decoded_pascal_string(codepage)?,
            ..Self::default()
        };

        Condition::read(&mut reader, codepage, version)?;

        if ((4, 0, 11)..(4, 1, 0)).contains(&version) {
            directory.permissions = reader.read_pascal_string()?;
        }

        if version >= (2, 0, 11) {
            directory.attributes = reader.read_u32::<LE>()?;
        }

        WindowsVersionRange::read_from(&mut reader, version)?;

        if version >= 4.1 {
            directory.permission = reader.read_i16::<LE>()?;
        }

        directory.flags = DirectoryFlags::from_bits_retain(reader.read_u8()?);

        Ok(directory)
    }

    /// Returns the name of the directory.
    #[must_use]
    #[inline]
    pub fn name(&self) -> Option<&str> {
        self.name.as_deref()
    }

    /// Returns the permissions of the directory.
    #[must_use]
    #[inline]
    pub fn permissions(&self) -> Option<&str> {
        self.permissions.as_ref().map(PascalString::as_str)
    }

    /// Returns the attributes of the directory.
    #[must_use]
    #[inline]
    pub const fn attributes(&self) -> u32 {
        self.attributes
    }

    /// Returns the permission index of the directory.
    #[must_use]
    #[inline]
    pub const fn permission(&self) -> i16 {
        self.permission
    }

    /// Returns the flags of the directory.
    #[must_use]
    #[inline]
    pub const fn flags(&self) -> DirectoryFlags {
        self.flags
    }
}

impl Default for Directory {
    fn default() -> Self {
        Self {
            name: None,
            permissions: None,
            attributes: 0,
            permission: -1,
            flags: DirectoryFlags::default(),
        }
    }
}

bitflags! {
    #[derive(Clone, Copy, Default, Eq, PartialEq)]
    pub struct DirectoryFlags: u8 {
        const NEVER_UNINSTALL = 1;
        const DELETE_AFTER_INSTALL = 1 << 1;
        const ALWAYS_UNINSTALL = 1 << 2;
        const SET_NTFS_COMPRESSION = 1 << 3;
        const UNSET_NTFS_COMPRESSION = 1 << 4;
    }
}

impl fmt::Debug for DirectoryFlags {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.is_empty() {
            f.write_str("None")
        } else {
            bitflags::parser::to_writer(self, f)
        }
    }
}

impl fmt::Display for DirectoryFlags {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        bitflags::parser::to_writer(self, f)
    }
}
