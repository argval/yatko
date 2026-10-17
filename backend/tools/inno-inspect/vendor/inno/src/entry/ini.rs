use std::{borrow::Cow, fmt, io};

use bitflags::bitflags;
use encoding_rs::Encoding;
use zerocopy::LE;

use super::Condition;
use crate::{InnoVersion, ReadBytesExt, WindowsVersionRange};

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct Ini {
    file: Cow<'static, str>,
    section: Option<String>,
    key: Option<String>,
    value: Option<String>,
    flags: IniFlags,
}

impl Ini {
    const DEFAULT_FILE: &'static str = "{windows}/WIN.INI";

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

        let mut ini = Self {
            file: reader
                .read_decoded_pascal_string(codepage)?
                .map_or(Cow::Borrowed(Self::DEFAULT_FILE), Cow::Owned),
            section: reader.read_decoded_pascal_string(codepage)?,
            key: reader.read_decoded_pascal_string(codepage)?,
            value: reader.read_decoded_pascal_string(codepage)?,
            ..Self::default()
        };

        Condition::read(&mut reader, codepage, version)?;

        WindowsVersionRange::read_from(&mut reader, version)?;

        ini.flags = IniFlags::from_bits_retain(reader.read_u8()?);

        Ok(ini)
    }

    /// Returns the file path for the INI entry.
    #[must_use]
    #[inline]
    pub fn file_path(&self) -> &str {
        &self.file
    }

    /// Returns the section name for the INI entry.
    #[must_use]
    #[inline]
    pub fn section_name(&self) -> Option<&str> {
        self.section.as_deref()
    }

    /// Returns the key name for the INI entry.
    #[must_use]
    #[inline]
    pub fn key_name(&self) -> Option<&str> {
        self.key.as_deref()
    }

    /// Returns the value for the INI entry.
    #[must_use]
    #[inline]
    pub fn value(&self) -> Option<&str> {
        self.value.as_deref()
    }

    /// Returns the flags for the INI entry.
    #[must_use]
    #[inline]
    pub const fn flags(&self) -> IniFlags {
        self.flags
    }
}

bitflags! {
    #[derive(Copy, Clone, Debug, Default, Eq, PartialEq)]
    pub struct IniFlags: u8 {
        const CREATE_KEY_IF_DOESNT_EXIST = 1;
        const UNINSTALL_DELETE_ENTRY = 1 << 1;
        const UNINSTALL_DELETE_ENTIRE_SECTION = 1 << 2;
        const UNINSTALL_DELETE_SECTION_IF_EMPTY = 1 << 3;
        const HAS_VALUE = 1 << 4;
    }
}

impl fmt::Display for IniFlags {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        bitflags::parser::to_writer(self, f)
    }
}
