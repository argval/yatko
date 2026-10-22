use std::{fmt, io};

use bitflags::bitflags;
use encoding_rs::Encoding;
use zerocopy::LE;

use crate::{
    read::ReadBytesExt,
    version::{InnoVersion, windows_version::WindowsVersionRange},
};

/// <https://github.com/jrsoftware/issrc/blob/is-6_5_1/Projects/Src/Shared.Struct.pas#L189>
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Component {
    name: Option<String>,
    description: Option<String>,
    types: Option<String>,
    languages: Option<String>,
    check_once: Option<String>,
    extra_disk_space_required: u64,
    level: u32,
    used: bool,
    flags: ComponentFlags,
    size: u64,
}

impl Component {
    pub fn read<R>(
        mut reader: R,
        codepage: &'static Encoding,
        version: InnoVersion,
    ) -> io::Result<Self>
    where
        R: io::Read,
    {
        let mut component = Self {
            name: reader.read_decoded_pascal_string(codepage)?,
            description: reader.read_decoded_pascal_string(codepage)?,
            types: reader.read_decoded_pascal_string(codepage)?,
            ..Self::default()
        };

        if version >= (4, 0, 1) {
            component.languages = reader.read_decoded_pascal_string(codepage)?;
        }

        if version >= 4 || (version.is_isx() && version >= (1, 3, 24)) {
            component.check_once = reader.read_decoded_pascal_string(codepage)?;
        }

        component.extra_disk_space_required = if version >= 4 {
            reader.read_u64::<LE>()?
        } else {
            reader.read_u32::<LE>()?.into()
        };

        if version >= 6.7 {
            component.level = reader.read_u8()?.into();
        } else if version >= 4 || (version.is_isx() && version >= (3, 0, 3)) {
            component.level = reader.read_u32::<LE>()?;
        }

        if version >= 4 || (version.is_isx() && version >= (3, 0, 4)) {
            component.used = reader.read_u8()? != 0;
        }

        WindowsVersionRange::read_from(&mut reader, version)?;

        component.flags = ComponentFlags::from_bits_retain(reader.read_u8()?);

        if version >= 4 {
            component.size = reader.read_u64::<LE>()?;
        } else if version >= 2 || (version.is_isx() && version >= (1, 3, 24)) {
            component.size = u64::from(reader.read_u32::<LE>()?);
        }

        Ok(component)
    }

    /// Returns the name of the component as a string slice.
    #[must_use]
    #[inline]
    pub fn name(&self) -> Option<&str> {
        self.name.as_deref()
    }

    /// Returns the description of the component as a string slice.
    #[must_use]
    #[inline]
    pub fn description(&self) -> Option<&str> {
        self.description.as_deref()
    }

    /// Returns the types of the component as a string slice.
    #[must_use]
    #[inline]
    pub fn types(&self) -> Option<&str> {
        self.types.as_deref()
    }

    /// Returns the languages of the component as a string slice.
    #[must_use]
    #[inline]
    pub fn languages(&self) -> Option<&str> {
        self.languages.as_deref()
    }

    /// Returns the check once string of the component as a string slice.
    #[must_use]
    #[inline]
    pub fn check_once(&self) -> Option<&str> {
        self.check_once.as_deref()
    }

    /// Returns the extra disk space required by the component.
    #[must_use]
    #[inline]
    pub const fn extra_disk_space_required(&self) -> u64 {
        self.extra_disk_space_required
    }

    /// Returns the level of the component.
    #[must_use]
    #[inline]
    pub const fn level(&self) -> u32 {
        self.level
    }

    /// Returns whether the component is used.
    #[must_use]
    #[inline]
    pub const fn used(&self) -> bool {
        self.used
    }

    /// Returns the flags of the component.
    #[must_use]
    #[inline]
    pub const fn flags(&self) -> ComponentFlags {
        self.flags
    }

    /// Returns the size of the component.
    #[must_use]
    #[inline]
    pub const fn size(&self) -> u64 {
        self.size
    }
}

impl Default for Component {
    fn default() -> Self {
        Self {
            name: None,
            description: None,
            types: None,
            languages: None,
            check_once: None,
            extra_disk_space_required: 0,
            level: 0,
            used: true,
            flags: ComponentFlags::default(),
            size: 0,
        }
    }
}

bitflags! {
    #[derive(Copy, Clone, Eq, PartialEq, Debug, Default)]
    pub struct ComponentFlags: u8 {
        const FIXED = 1;
        const RESTART = 1 << 1;
        const DISABLE_NO_UNINSTALL_WARNING = 1 << 2;
        const EXCLUSIVE = 1 << 3;
        const DONT_INHERIT_CHECK = 1 << 4;
    }
}

impl fmt::Display for ComponentFlags {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        bitflags::parser::to_writer(self, f)
    }
}
