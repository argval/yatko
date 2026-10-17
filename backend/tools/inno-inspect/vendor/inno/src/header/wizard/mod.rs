mod color;
mod image_alpha_format;
mod light_control_styling;
mod style;
mod wizard_size_percent;

use std::io;

pub use color::Color;
pub use image_alpha_format::ImageAlphaFormat;
pub use light_control_styling::LightControlStyling;
pub use style::WizardStyle;
pub use wizard_size_percent::WizardSizePercent;

use crate::{read::ReadBytesExt, version::InnoVersion};

#[derive(Copy, Clone, Default, Eq, PartialEq)]
pub struct WizardSettings {
    image_alpha_format: ImageAlphaFormat,
    pub(crate) image_back_color: Color,
    pub(crate) small_image_back_color: Color,
    pub(crate) back_color: Color,
    pub(crate) image_back_color_dynamic_dark: Color,
    pub(crate) small_image_back_color_dynamic_dark: Color,
    pub(crate) back_color_dynamic_dark: Color,
    pub(crate) image_opacity: Option<u8>,
    pub(crate) back_image_opacity: Option<u8>,
    pub(crate) light_control_setting: Option<LightControlStyling>,
    size_percent: WizardSizePercent,
    style: WizardStyle,
}

impl WizardSettings {
    pub fn read_from<R>(mut reader: R, version: InnoVersion) -> io::Result<Self>
    where
        R: io::Read,
    {
        let mut wizard_settings = Self::default();

        if version < (5, 5, 7) {
            wizard_settings.image_back_color = reader.read_t::<Color>()?;
        }

        if ((2, 0, 0)..(5, 0, 4)).contains(&version) || version.is_isx() {
            wizard_settings.small_image_back_color = reader.read_t::<Color>()?;
        }

        if (6.0..6.6).contains(&version) {
            wizard_settings.style = WizardStyle::try_read_from(&mut reader, version)?;
        }

        if version >= 6 {
            wizard_settings.size_percent = reader.read_t::<WizardSizePercent>()?;
        }

        if version >= 6.6 {
            wizard_settings.style = WizardStyle::try_read_from(&mut reader, version)?;
        }

        if version >= (5, 5, 7) {
            wizard_settings.image_alpha_format = ImageAlphaFormat::try_read_from_io(&mut reader)?;
        }

        Ok(wizard_settings)
    }

    /// Returns the image alpha format.
    #[must_use]
    #[inline]
    pub const fn image_alpha_format(&self) -> ImageAlphaFormat {
        self.image_alpha_format
    }

    /// Returns the image background color.
    #[must_use]
    #[inline]
    pub const fn image_back_color(&self) -> Color {
        self.image_back_color
    }

    /// Returns the small image background color.
    #[must_use]
    #[inline]
    pub const fn small_image_back_color(&self) -> Color {
        self.small_image_back_color
    }

    /// Returns the image background color used in dark mode when a dynamic theme is enabled.
    #[must_use]
    #[inline]
    pub const fn image_back_color_dynamic_dark(&self) -> Color {
        self.image_back_color_dynamic_dark
    }

    /// Returns the small image background color used in dark mode when a dynamic theme is enabled.
    #[must_use]
    #[inline]
    pub const fn small_image_back_color_dynamic_dark(&self) -> Color {
        self.small_image_back_color_dynamic_dark
    }

    /// Returns the image opacity.
    #[must_use]
    #[inline]
    pub const fn image_opacity(&self) -> Option<u8> {
        self.image_opacity
    }

    /// Returns the background image opacity.
    ///
    /// Added in Inno Setup 6.7.0.
    #[must_use]
    #[inline]
    pub const fn back_image_opacity(&self) -> Option<u8> {
        self.back_image_opacity
    }

    /// Returns the wizard size percent
    #[must_use]
    #[inline]
    pub const fn size_percent(&self) -> WizardSizePercent {
        self.size_percent
    }

    /// Returns the wizard style.
    #[must_use]
    #[inline]
    pub const fn style(&self) -> WizardStyle {
        self.style
    }
}
