use encoding_rs::Encoding;

use crate::string::PascalString;

#[derive(Clone, Eq, PartialEq)]
pub struct DialogFont {
    /// The dialog font's name.
    pub(crate) name: PascalString,

    /// The font's size.
    pub(crate) size: u32,

    /// The font's standard height.
    pub(crate) standard_height: u32,

    /// The font's base scale height.
    ///
    /// Added in Inno Setup 6.6.0.
    pub(crate) base_scale_height: u32,

    /// The font's base scale width.
    ///
    /// Added in Inno Setup 6.6.0
    pub(crate) base_scale_width: u32,
}

impl DialogFont {
    /// Decodes the dialog font name using the specified codepage if it is not already decoded.
    pub(crate) fn decode(&mut self, codepage: &'static Encoding) {
        self.name.decode(codepage);
    }

    /// Returns the dialog font's name.
    #[must_use]
    #[inline]
    pub fn name(&self) -> &str {
        self.name.as_str()
    }

    /// Returns the dialog font's size.
    #[must_use]
    #[inline]
    pub const fn size(&self) -> u32 {
        self.size
    }

    /// Returns the dialog font's standard height.
    #[must_use]
    #[inline]
    pub const fn standard_height(&self) -> u32 {
        self.standard_height
    }

    /// Returns the dialog font's base scale height.
    #[must_use]
    #[inline]
    pub const fn base_scale_height(&self) -> u32 {
        self.base_scale_height
    }

    /// Returns the dialog font's base scale width.
    #[must_use]
    #[inline]
    pub const fn base_scale_width(&self) -> u32 {
        self.base_scale_width
    }
}

impl Default for DialogFont {
    /// <https://github.com/jrsoftware/issrc/blob/is-6_6_1/Projects/Src/Compiler.SetupCompiler.pas#L6330>
    fn default() -> Self {
        Self {
            name: PascalString::from("Segoe UI"),
            size: 9,
            standard_height: 0,
            base_scale_width: 7,
            base_scale_height: 15,
        }
    }
}
