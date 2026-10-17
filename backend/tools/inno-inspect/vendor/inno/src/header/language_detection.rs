use std::io;

use zerocopy::{Immutable, KnownLayout, TryFromBytes, ValidityError, try_transmute};

use super::HeaderFlags;

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum LanguageDetection {
    #[default]
    UILanguage = 0,
    LocaleLanguage = 1,
    None = 2,
}

impl LanguageDetection {
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

    /// Returns the `LanguageDetection` as a static string.
    #[must_use]
    pub const fn as_str(&self) -> &'static str {
        match self {
            Self::UILanguage => "UILanguage",
            Self::LocaleLanguage => "LocaleLanguage",
            Self::None => "None",
        }
    }
}

impl From<HeaderFlags> for LanguageDetection {
    fn from(flags: HeaderFlags) -> Self {
        if flags.contains(HeaderFlags::DETECT_LANGUAGE_USING_LOCALE) {
            Self::LocaleLanguage
        } else {
            Self::UILanguage
        }
    }
}

impl TryFrom<u8> for LanguageDetection {
    type Error = ValidityError<u8, Self>;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        try_transmute!(value)
    }
}
