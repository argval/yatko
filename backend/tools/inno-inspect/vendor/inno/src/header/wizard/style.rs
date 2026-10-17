use std::{fmt, io};

use zerocopy::{Immutable, KnownLayout, TryFromBytes};

use crate::{read::ReadBytesExt, version::InnoVersion};

/// <https://jrsoftware.org/ishelp/index.php?topic=setup_wizardstyle>
///
/// <https://github.com/jrsoftware/issrc/blob/is-6_6_0/Projects/Src/Shared.Struct.pas#L84>
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum WizardStyle {
    #[default]
    Light,
    Dark,
    Dynamic,

    Classic,
    Modern,
}

impl WizardStyle {
    pub fn try_read_from<R>(mut reader: R, version: InnoVersion) -> io::Result<Self>
    where
        Self: Sized,
        R: io::Read,
    {
        let value = reader.read_u8()?;

        if version >= 6.6 {
            match value {
                0 => return Ok(Self::Light),
                1 => return Ok(Self::Dark),
                2 => return Ok(Self::Dynamic),
                _ => {}
            }
        } else {
            match value {
                0 => return Ok(Self::Classic),
                1 => return Ok(Self::Modern),
                _ => {}
            }
        }

        Err(io::Error::new(
            io::ErrorKind::InvalidData,
            format!("Unknown Wizard style value: {value}"),
        ))
    }

    /// Returns the `InnoStyle` as a static string.
    #[must_use]
    pub const fn as_str(&self) -> &'static str {
        match self {
            Self::Light => "Light",
            Self::Dark => "Dark",
            Self::Dynamic => "Dynamic",
            Self::Classic => "Classic",
            Self::Modern => "Modern",
        }
    }
}

impl fmt::Display for WizardStyle {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}
