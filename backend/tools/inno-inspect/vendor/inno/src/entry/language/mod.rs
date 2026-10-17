mod dialog_font;

use std::{fmt, io};

pub use dialog_font::DialogFont;
use encoding_rs::{Encoding, UTF_16LE, WINDOWS_1252};
use zerocopy::LE;

use crate::{read::ReadBytesExt, string::PascalString, version::InnoVersion};

/// <https://github.com/jrsoftware/issrc/blob/is-6_6_1/Projects/Src/Shared.Struct.pas#L152>
///
/// <https://github.com/jrsoftware/issrc/blob/is-6_6_1/Projects/Src/Shared.LangOptionsSectionDirectives.pas>
#[derive(Clone, Eq, PartialEq)]
pub struct Language {
    /// The internal name of the language in English.
    name: PascalString,
    /// The name of the language in the language itself.
    language_name: PascalString,
    /// The dialog font.
    dialog_font: DialogFont,
    title_font: PascalString,     // Obsolete
    title_font_size: u32,         // Obsolete
    copyright_font: PascalString, // Obsolete
    copyright_font_size: u32,     // Obsolete
    welcome_font: PascalString,
    welcome_font_size: u32,
    data: PascalString,
    license_text: PascalString,
    info_before: PascalString,
    info_after: PascalString,
    id: u32,
    codepage: &'static Encoding,
    right_to_left: bool,
}

impl Language {
    pub fn read<R>(mut reader: R, version: InnoVersion) -> io::Result<Self>
    where
        R: io::Read,
    {
        let mut language = Self::default();

        if version >= 4
            && let Some(internal_name) = reader.read_pascal_string()?
        {
            language.name = internal_name;
        }

        if let Some(language_name) = reader.read_pascal_string()? {
            language.language_name = language_name;
        }

        if version == (5, 5, 7, 1) {
            reader.discard_pascal_string()?;
        }

        if let Some(dialog_font) = reader.read_pascal_string()? {
            language.dialog_font.name = dialog_font;
        }

        if version < 6.6
            && let Some(title_font) = reader.read_pascal_string()?
        {
            language.title_font = title_font;
        }

        if let Some(welcome_font) = reader.read_pascal_string()? {
            language.welcome_font = welcome_font;
        }

        if version < 6.6
            && let Some(copyright_font) = reader.read_pascal_string()?
        {
            language.copyright_font = copyright_font;
        }

        if version >= 4
            && let Some(data) = reader.read_pascal_string()?
        {
            language.data = data;
        }

        if version >= (4, 0, 1) {
            if let Some(license_text) = reader.read_pascal_string()? {
                language.license_text = license_text;
            }
            if let Some(info_before) = reader.read_pascal_string()? {
                language.info_before = info_before;
            }
            if let Some(info_after) = reader.read_pascal_string()? {
                language.info_after = info_after;
            }
        }

        language.id = if version >= 6.6 {
            reader.read_u16::<LE>()?.into()
        } else {
            reader.read_u32::<LE>()?
        };

        if version < (4, 2, 2) {
            if let Ok(codepage) = u16::try_from(language.id)
                && let Some(encoding) = codepage::to_encoding(codepage)
            {
                language.codepage = encoding;
            }
        } else if !version.is_unicode() {
            let codepage = reader.read_u32::<LE>()?;
            if codepage != 0
                && let Ok(codepage) = u16::try_from(codepage)
                && let Some(encoding) = codepage::to_encoding(codepage)
            {
                language.codepage = encoding;
            }
        } else {
            if version < 5.3 {
                reader.read_u32::<LE>()?;
            }
            language.codepage = UTF_16LE;
        }

        // Now that we have the codepage, decode the earlier strings
        language.name.decode(language.codepage);
        language.language_name.decode(language.codepage);
        language.dialog_font.decode(language.codepage);
        language.title_font.decode(language.codepage);
        language.welcome_font.decode(language.codepage);
        language.copyright_font.decode(language.codepage);

        language.dialog_font.size = reader.read_u32::<LE>()?;

        if version < 4.1 {
            language.dialog_font.standard_height = reader.read_u32::<LE>()?;
        }

        if version >= 6.6 {
            language.dialog_font.base_scale_height = reader.read_u32::<LE>()?;
            language.dialog_font.base_scale_width = reader.read_u32::<LE>()?;
        } else {
            language.title_font_size = reader.read_u32::<LE>()?;
        }

        language.welcome_font_size = reader.read_u32::<LE>()?;

        if version < 6.6 {
            language.copyright_font_size = reader.read_u32::<LE>()?;
        }

        if version == (5, 5, 7, 1) {
            reader.read_u32::<LE>()?;
        }

        if version >= (5, 2, 3) {
            language.right_to_left = reader.read_u8()? != 0;
        }

        Ok(language)
    }

    /// Returns the internal name of the language in English.
    #[must_use]
    #[inline]
    pub fn name(&self) -> &str {
        self.name.as_str()
    }

    /// Returns the name of the language in the language itself.
    #[must_use]
    #[inline]
    pub fn language_name(&self) -> &str {
        self.language_name.as_str()
    }

    /// Returns the dialog font.
    #[must_use]
    #[inline]
    pub const fn dialog_font(&self) -> &DialogFont {
        &self.dialog_font
    }

    /// Returns the title font name.
    #[must_use]
    #[inline]
    pub fn title_font(&self) -> &str {
        self.title_font.as_str()
    }

    /// Returns the welcome font name.
    #[must_use]
    #[inline]
    pub fn welcome_font(&self) -> &str {
        self.welcome_font.as_str()
    }

    /// Returns the copyright font name.
    #[must_use]
    #[inline]
    pub fn copyright_font(&self) -> &str {
        self.copyright_font.as_str()
    }

    /// Returns the data associated with the language, if available.
    #[must_use]
    #[inline]
    pub fn data(&self) -> &str {
        self.data.as_str()
    }

    /// Returns the license text, if available.
    #[must_use]
    #[inline]
    pub fn license_text(&self) -> &str {
        self.license_text.as_str()
    }

    /// Returns the info text before the license, if available.
    #[must_use]
    #[inline]
    pub fn info_before(&self) -> &str {
        self.info_before.as_str()
    }

    /// Returns the info text after the license, if available.
    #[must_use]
    #[inline]
    pub fn info_after(&self) -> &str {
        self.info_after.as_str()
    }

    /// Returns the language ID.
    #[must_use]
    #[inline]
    pub const fn id(&self) -> u32 {
        self.id
    }

    /// Returns the codepage used by the language.
    #[must_use]
    #[inline]
    pub const fn codepage(&self) -> &'static Encoding {
        self.codepage
    }

    /// Returns the title font size.
    #[must_use]
    #[inline]
    pub const fn title_font_size(&self) -> u32 {
        self.title_font_size
    }

    /// Returns the welcome font size.
    #[must_use]
    #[inline]
    pub const fn welcome_font_size(&self) -> u32 {
        self.welcome_font_size
    }

    /// Returns the copyright font size.
    #[must_use]
    #[inline]
    pub const fn copyright_font_size(&self) -> u32 {
        self.copyright_font_size
    }

    /// Returns whether the language is right-to-left.
    #[doc(alias = "rtl")]
    #[must_use]
    #[inline]
    pub const fn right_to_left(&self) -> bool {
        self.right_to_left
    }
}

impl fmt::Debug for Language {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        f.debug_struct("Language")
            .field("Name", &self.name())
            .field("LanguageName", &self.language_name())
            .field("DialogFontName", &self.dialog_font().name())
            .field("TitleFontName", &self.title_font())
            .field("WelcomeFontName", &self.welcome_font())
            .field("CopyrightFontName", &self.copyright_font())
            .field("Data", &self.data())
            .field("LicenseText", &self.license_text())
            .field("InfoBefore", &self.info_before())
            .field("InfoAfter", &self.info_after())
            .field("LanguageID", &self.id())
            .field("Codepage", &self.codepage())
            .field("DialogFontSize", &self.dialog_font().size())
            .field(
                "DialogFontStandardHeight",
                &self.dialog_font().standard_height(),
            )
            .field("TitleFontSize", &self.title_font_size())
            .field("WelcomeFontSize", &self.welcome_font_size())
            .field("CopyrightFontSize", &self.copyright_font_size())
            .field("RightToLeft", &self.right_to_left())
            .finish()
    }
}

impl Default for Language {
    /// <https://github.com/jrsoftware/issrc/blob/is-6_6_1/Projects/Src/Compiler.SetupCompiler.pas#L6323>
    fn default() -> Self {
        Self {
            name: PascalString::from("default"),
            language_name: PascalString::from("English"),
            dialog_font: DialogFont::default(),
            title_font: PascalString::from("Arial"),
            title_font_size: 29,
            welcome_font: PascalString::from("Segoe UI"),
            welcome_font_size: 14,
            copyright_font: PascalString::from("Arial"),
            copyright_font_size: 8,
            data: PascalString::default(),
            license_text: PascalString::default(),
            info_before: PascalString::default(),
            info_after: PascalString::default(),
            id: 1033, // English (United States)
            codepage: WINDOWS_1252,
            right_to_left: false,
        }
    }
}
