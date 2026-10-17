use std::{fmt, io};

use zerocopy::LE;

use super::{
    header::{Compression, Header, HeaderFlags},
    read::ReadBytesExt,
    version::InnoVersion,
};

#[derive(Default)]
pub struct Wizard {
    images: Vec<Vec<u8>>,
    small_images: Vec<Vec<u8>>,
    back_images: Vec<Vec<u8>>,
    images_dynamic_dark: Vec<Vec<u8>>,
    small_images_dynamic_dark: Vec<Vec<u8>>,
    back_images_dynamic_dark: Vec<Vec<u8>>,
    decompressor_dll: Vec<u8>,
    decrypt_dll: Vec<u8>,
}

impl Wizard {
    pub fn read<R>(mut reader: R, header: &Header, version: InnoVersion) -> io::Result<Self>
    where
        R: io::Read,
    {
        let mut wizard = Self {
            images: Self::read_images(&mut reader, version)?,
            ..Self::default()
        };

        if version >= 2 || version.is_isx() {
            wizard.small_images = Self::read_images(&mut reader, version)?;
        }

        if version >= 6.7 {
            wizard.back_images = Self::read_images(&mut reader, version)?;
        }

        if version >= 6.6 {
            wizard.images_dynamic_dark = Self::read_images(&mut reader, version)?;
            wizard.small_images_dynamic_dark = Self::read_images(&mut reader, version)?;
        }

        if version >= 6.7 {
            wizard.back_images_dynamic_dark = Self::read_images(&mut reader, version)?;
        }

        if header.compression() == Compression::BZip2
            || (header.compression() == Compression::LZMA1 && version == (4, 1, 5))
            || (header.compression() == Compression::Zlib && version >= (4, 2, 6))
        {
            wizard.decompressor_dll = reader.read_raw_pascal_string()?;
        }

        if header.flags.contains(HeaderFlags::ENCRYPTION_USED) {
            wizard.decrypt_dll = reader.read_raw_pascal_string()?;
        }

        Ok(wizard)
    }

    fn read_images<R>(mut reader: R, version: InnoVersion) -> io::Result<Vec<Vec<u8>>>
    where
        R: io::Read,
    {
        let count = if version >= 5.6 {
            reader.read_i32::<LE>()?
        } else {
            1
        };

        if count == -1 {
            return Ok(Vec::new());
        }

        let mut images = (0..count)
            .map(|_| reader.read_raw_pascal_string())
            .collect::<io::Result<Vec<_>>>()?;

        if version < 5.6 && images.first().is_some_and(Vec::is_empty) {
            images.clear();
        }

        Ok(images)
    }

    /// Returns the images used in the [Wizard].
    #[must_use]
    #[inline]
    pub fn images(&self) -> &[Vec<u8>] {
        &self.images
    }

    /// Returns the small images used in the [Wizard].
    #[must_use]
    #[inline]
    pub fn small_images(&self) -> &[Vec<u8>] {
        &self.small_images
    }

    /// Returns the back images used in the [Wizard].
    #[must_use]
    #[inline]
    pub fn back_images(&self) -> &[Vec<u8>] {
        &self.back_images
    }

    /// Returns the dynamic dark variant images used in the [Wizard].
    #[must_use]
    #[inline]
    pub fn images_dynamic_dark(&self) -> &[Vec<u8>] {
        &self.images_dynamic_dark
    }

    /// Returns the dynamic dark variant small images used in the [Wizard].
    #[must_use]
    #[inline]
    pub fn small_images_dynamic_dark(&self) -> &[Vec<u8>] {
        &self.small_images_dynamic_dark
    }

    /// Returns the dynamic dark variant back images used in the [Wizard].
    #[must_use]
    #[inline]
    pub fn back_images_dynamic_dark(&self) -> &[Vec<u8>] {
        &self.back_images_dynamic_dark
    }

    /// Returns the decompressor DLL, if present.
    #[must_use]
    #[inline]
    pub fn decompressor_dll(&self) -> &[u8] {
        &self.decompressor_dll
    }

    /// Returns the decrypt DLL, if present.
    #[must_use]
    #[inline]
    pub fn decrypt_dll(&self) -> &[u8] {
        &self.decrypt_dll
    }
}

impl fmt::Debug for Wizard {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        // Create a best-fit debug representation without dumping the image's bytes.
        f.debug_struct("Wizard")
            .field(
                "Images (image lengths)",
                &self.images().iter().map(Vec::len).collect::<Vec<_>>(),
            )
            .field(
                "SmallImages (image lengths)",
                &self.small_images().iter().map(Vec::len).collect::<Vec<_>>(),
            )
            .field(
                "BackImages (image lengths)",
                &self.back_images().iter().map(Vec::len).collect::<Vec<_>>(),
            )
            .field(
                "Images Dynamic Dark (image lengths)",
                &self
                    .images_dynamic_dark()
                    .iter()
                    .map(Vec::len)
                    .collect::<Vec<_>>(),
            )
            .field(
                "SmallImages Dynamic Dark (image lengths)",
                &self
                    .small_images_dynamic_dark()
                    .iter()
                    .map(Vec::len)
                    .collect::<Vec<_>>(),
            )
            .field(
                "BackImages Dynamic Dark (image lengths)",
                &self
                    .back_images_dynamic_dark()
                    .iter()
                    .map(Vec::len)
                    .collect::<Vec<_>>(),
            )
            .field("DecompressorDLL (length)", &self.decompressor_dll().len())
            .field("DecryptDLL (length)", &self.decrypt_dll().len())
            .finish()
    }
}
