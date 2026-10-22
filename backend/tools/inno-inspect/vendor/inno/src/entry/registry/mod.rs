mod flags;
mod root;
mod r#type;

use std::io;

use encoding_rs::Encoding;
pub use flags::RegistryFlags;
pub use root::RegRoot;
pub use r#type::RegistryValueType;
use zerocopy::{LE, try_transmute};

use super::{Bitness, Condition};
use crate::{
    InnoVersion, ReadBytesExt, WindowsVersionRange, header::flag_reader::read_flags::read_flags,
    string::PascalString,
};

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RegistryEntry {
    key: Option<String>,
    name: Option<String>,
    value: Option<PascalString>,
    permissions: Option<PascalString>,
    reg_root: RegRoot,
    permission: i16,
    r#type: RegistryValueType,
    bitness: Bitness,
    flags: RegistryFlags,
}

impl RegistryEntry {
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

        let mut registry = Self {
            key: reader.read_decoded_pascal_string(codepage)?,
            name: reader.read_decoded_pascal_string(codepage)?,
            value: reader.read_pascal_string()?,
            ..Self::default()
        };

        Condition::read(&mut reader, codepage, version)?;

        if ((4, 0, 11)..(4, 1, 0)).contains(&version) {
            registry.permissions = reader.read_pascal_string()?;
        }

        WindowsVersionRange::read_from(&mut reader, version)?;

        registry.reg_root =
            try_transmute!(reader.read_u32::<LE>()? & !0x8000_0000).unwrap_or_default();

        if version >= 4.1 {
            registry.permission = reader.read_i16::<LE>()?;
        }

        registry.r#type = RegistryValueType::try_read_from_io(&mut reader)?;

        if version >= (7, 0, 0, 3) {
            registry.bitness = Bitness::try_read_from_io(&mut reader)?;
        }

        registry.flags = read_flags!(&mut reader,
            [
                RegistryFlags::CREATE_VALUE_IF_DOESNT_EXIST,
                RegistryFlags::UNINSTALL_DELETE_VALUE,
                RegistryFlags::UNINSTALL_CLEAR_VALUE,
                RegistryFlags::UNINSTALL_DELETE_ENTIRE_KEY,
                RegistryFlags::UNINSTALL_DELETE_ENTIRE_KEY_IF_EMPTY,
            ],
            if version >= (1, 2, 6) => RegistryFlags::PRESERVE_STRING_TYPE,
            if version >= (1, 3, 9) => [RegistryFlags::DELETE_KEY, RegistryFlags::DELETE_VALUE],
            if version >= (1, 3, 12) => RegistryFlags::NO_ERROR,
            if version >= (1, 3, 16) => RegistryFlags::DONT_CREATE_KEY,
            if version >= 5.1 && version < (7, 0, 0, 3) => [RegistryFlags::BITS_32, RegistryFlags::BITS_64]
        )?;

        if registry.bitness == Bitness::Bit32 {
            registry.flags |= RegistryFlags::BITS_32;
        } else if registry.bitness == Bitness::Bit64 {
            registry.flags |= RegistryFlags::BITS_64;
        }

        Ok(registry)
    }

    /// Returns the registry key name as a string slice.
    #[must_use]
    #[inline]
    pub fn key(&self) -> Option<&str> {
        self.key.as_deref()
    }

    /// Returns the registry value name as a string slice.
    #[must_use]
    #[inline]
    pub fn name(&self) -> Option<&str> {
        self.name.as_deref()
    }

    /// Returns the registry value as a string slice.
    #[must_use]
    pub fn value(&self) -> Option<&str> {
        self.value.as_ref().map(PascalString::as_str)
    }

    /// Returns the permissions as a string slice.
    #[must_use]
    pub fn permissions(&self) -> Option<&str> {
        self.permissions.as_ref().map(PascalString::as_str)
    }

    /// Returns the registry root.
    #[must_use]
    #[inline]
    pub const fn registry_root(&self) -> RegRoot {
        self.reg_root
    }

    /// Returns the permission index.
    #[must_use]
    #[inline]
    pub const fn permission(&self) -> i16 {
        self.permission
    }

    /// Returns the registry value type.
    #[must_use]
    #[inline]
    pub const fn r#type(&self) -> RegistryValueType {
        self.r#type
    }

    /// Returns the bitness of the registry entry.
    #[must_use]
    #[inline]
    pub const fn bitness(&self) -> Bitness {
        self.bitness
    }

    /// Returns the registry flags.
    #[must_use]
    #[inline]
    pub const fn flags(&self) -> RegistryFlags {
        self.flags
    }
}

impl Default for RegistryEntry {
    fn default() -> Self {
        Self {
            key: None,
            name: None,
            value: None,
            permissions: None,
            reg_root: RegRoot::default(),
            permission: -1,
            r#type: RegistryValueType::default(),
            bitness: Bitness::default(),
            flags: RegistryFlags::default(),
        }
    }
}
