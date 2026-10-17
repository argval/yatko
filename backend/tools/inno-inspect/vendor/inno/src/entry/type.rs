use std::{fmt, io};

use bitflags::bitflags;
use encoding_rs::Encoding;
use zerocopy::{Immutable, KnownLayout, LE, TryFromBytes};

use crate::{InnoVersion, read::ReadBytesExt, version::windows_version::WindowsVersionRange};

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct Type {
    name: Option<String>,
    description: Option<String>,
    languages: Option<String>,
    check: Option<String>,
    is_custom: bool,
    setup: SetupType,
    size: u64,
}

impl Type {
    pub fn read<R>(
        mut reader: R,
        codepage: &'static Encoding,
        version: InnoVersion,
    ) -> io::Result<Self>
    where
        R: io::Read,
    {
        let mut r#type = Self {
            name: reader.read_decoded_pascal_string(codepage)?,
            description: reader.read_decoded_pascal_string(codepage)?,
            ..Self::default()
        };

        if version >= (4, 0, 1) {
            r#type.languages = reader.read_decoded_pascal_string(codepage)?;
        }

        if version >= 4 || (version.is_isx() && version >= (1, 3, 24)) {
            r#type.check = reader.read_decoded_pascal_string(codepage)?;
        }

        WindowsVersionRange::read_from(&mut reader, version)?;

        let flags = TypeFlags::from_bits_retain(reader.read_u8()?);
        r#type.is_custom = flags.contains(TypeFlags::CUSTOM_SETUP_TYPE);

        if version >= (4, 0, 3) {
            r#type.setup = SetupType::try_read_from_io(&mut reader)?;
        }

        r#type.size = if version >= 4 {
            reader.read_u64::<LE>()?
        } else {
            reader.read_u32::<LE>()?.into()
        };

        Ok(r#type)
    }

    /// Returns the name of the type as a string slice.
    #[must_use]
    #[inline]
    pub fn name(&self) -> Option<&str> {
        self.name.as_deref()
    }

    /// Returns the description of the type as a string slice.
    #[must_use]
    #[inline]
    pub fn description(&self) -> Option<&str> {
        self.description.as_deref()
    }

    /// Returns the languages of the type as a string slice.
    #[must_use]
    #[inline]
    pub fn languages(&self) -> Option<&str> {
        self.languages.as_deref()
    }

    /// Returns the check string of the type as a string slice.
    #[must_use]
    #[inline]
    pub fn check(&self) -> Option<&str> {
        self.check.as_deref()
    }

    /// Returns whether the type is custom.
    #[must_use]
    #[inline]
    pub const fn is_custom(&self) -> bool {
        self.is_custom
    }

    /// Returns the setup type of the type.
    #[must_use]
    #[inline]
    pub const fn setup(&self) -> SetupType {
        self.setup
    }

    /// Returns the size of the type.
    #[must_use]
    #[inline]
    pub const fn size(&self) -> u64 {
        self.size
    }
}

#[derive(Copy, Clone, Eq, PartialEq, Debug, Default, TryFromBytes, KnownLayout, Immutable)]
#[repr(u8)]
pub enum SetupType {
    #[default]
    User = 0,
    DefaultFull = 1,
    DefaultCompact = 2,
    DefaultCustom = 3,
}

impl SetupType {
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

    pub const fn as_str(self) -> &'static str {
        match self {
            Self::User => "User",
            Self::DefaultFull => "Default full",
            Self::DefaultCompact => "Default compact",
            Self::DefaultCustom => "Default custom",
        }
    }
}

impl fmt::Display for SetupType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}

bitflags! {
    #[derive(Clone, Copy, Debug, Default)]
    pub struct TypeFlags: u8 {
        const CUSTOM_SETUP_TYPE = 1;
    }
}
