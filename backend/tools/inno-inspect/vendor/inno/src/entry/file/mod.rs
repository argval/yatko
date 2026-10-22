mod copy_mode;
mod flags;
mod r#type;
mod verification;

use std::io;

pub use copy_mode::FileCopyMode;
use encoding_rs::Encoding;
pub use flags::FileFlags;
pub use r#type::FileType;
pub use verification::FileVerification;
use zerocopy::LE;

use crate::{
    entry::{Bitness, Condition},
    header::flag_reader::read_flags::read_flags,
    read::ReadBytesExt,
    version::{InnoVersion, windows_version::WindowsVersionRange},
};

/// <https://github.com/jrsoftware/issrc/blob/is-7_0_0/Projects/Src/Shared.Struct.pas#L282>
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct File {
    source: Option<String>,
    destination: Option<String>,
    install_font_name: Option<String>,
    strong_assembly_name: Option<String>,
    excludes: Option<String>,
    download_is_sig_source: Option<String>,
    download_user_name: Option<String>,
    download_password: Option<String>,
    extract_archive_password: Option<String>,
    verification: Option<FileVerification>,
    condition: Condition,
    /// Index into the file location entry list
    location: u32,
    attributes: u32,
    external_size: u64,
    /// Index into the permission entry list
    permission: i16,
    bitness: Bitness,
    flags: FileFlags,
    r#type: FileType,
}

impl File {
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

        let mut file = Self {
            source: reader.read_decoded_pascal_string(codepage)?,
            destination: reader.read_decoded_pascal_string(codepage)?,
            install_font_name: reader.read_decoded_pascal_string(codepage)?,
            ..Self::default()
        };

        if version >= (5, 2, 5) {
            file.strong_assembly_name = reader.read_decoded_pascal_string(codepage)?;
        }

        file.condition = Condition::read(&mut reader, codepage, version)?;

        if version >= 6.5 {
            file.excludes = reader.read_decoded_pascal_string(codepage)?;
            file.download_is_sig_source = reader.read_decoded_pascal_string(codepage)?;
            file.download_user_name = reader.read_decoded_pascal_string(codepage)?;
            file.download_password = reader.read_decoded_pascal_string(codepage)?;
            file.extract_archive_password = reader.read_decoded_pascal_string(codepage)?;
            file.verification = Some(FileVerification::read(&mut reader, codepage)?);
        }

        WindowsVersionRange::read_from(&mut reader, version)?;

        file.location = reader.read_u32::<LE>()?;
        file.attributes = reader.read_u32::<LE>()?;
        file.external_size = if version >= 4 {
            reader.read_u64::<LE>()?
        } else {
            reader.read_u32::<LE>()?.into()
        };

        if version < (3, 0, 5) {
            file.flags |= FileFlags::from(FileCopyMode::try_read_from_io(&mut reader)?);
        }

        if version >= 4.1 {
            file.permission = reader.read_i16::<LE>()?;
        }

        if version >= (7, 0, 0, 3) {
            file.bitness = Bitness::try_read_from_io(&mut reader)?;
        }

        file.flags |= read_flags!(&mut reader,
            [
                FileFlags::CONFIRM_OVERWRITE,
                FileFlags::NEVER_UNINSTALL,
                FileFlags::RESTART_REPLACE,
                FileFlags::DELETE_AFTER_INSTALL,
                FileFlags::REGISTER_SERVER,
                FileFlags::REGISTER_TYPE_LIB,
                FileFlags::SHARED_FILE,
            ],
            if version < 2 && !version.is_isx() => FileFlags::IS_README_FILE,
            [FileFlags::COMPARE_TIME_STAMP, FileFlags::FONT_IS_NOT_TRUE_TYPE],
            if version >= (1, 2, 5) => FileFlags::SKIP_IF_SOURCE_DOESNT_EXIST,
            if version >= (1, 2, 6) => FileFlags::OVERWRITE_READ_ONLY,
            if version >= (1, 3, 21) => [
                FileFlags::OVERWRITE_SAME_VERSION,
                FileFlags::CUSTOM_DEST_NAME
            ],
            if version >= (1, 3, 25) => FileFlags::ONLY_IF_DEST_FILE_EXISTS,
            if version >= (2, 0, 5) => FileFlags::NO_REG_ERROR,
            if version >= (3, 0, 1) => FileFlags::UNINS_RESTART_DELETE,
            if version >= (3, 0, 5) => [
                FileFlags::ONLY_IF_DOESNT_EXIST,
                FileFlags::IGNORE_VERSION,
                FileFlags::PROMPT_IF_OLDER,
            ],
            if version >= 4 || (version.is_isx() && version >= (3, 0, 6)) => FileFlags::DONT_COPY,
            if version >= (4, 0, 5) => FileFlags::UNINS_REMOVE_READ_ONLY,
            if version >= (4, 1, 8) => FileFlags::RECURSE_SUB_DIRS_EXTERNAL,
            if version >= (4, 2, 1) => FileFlags::REPLACE_SAME_VERSION_IF_CONTENTS_DIFFER,
            if version >= (4, 2, 5) => FileFlags::DONT_VERIFY_CHECKSUM,
            if version >= (5, 0, 3) => FileFlags::UNINS_NO_SHARED_FILE_PROMPT,
            if version >= 5.1 => FileFlags::CREATE_ALL_SUB_DIRS,
            if version >= (5, 1, 2) && version < (7, 0, 0, 3) => FileFlags::BITS_32, FileFlags::BITS_64,
            if version >= 5.2 => [
                FileFlags::EXTERNAL_SIZE_PRESET,
                FileFlags::SET_NTFS_COMPRESSION,
                FileFlags::UNSET_NTFS_COMPRESSION,
            ],
            if version >= (5, 2, 5) => FileFlags::GAC_INSTALL,
            if version >= 6.5 => [FileFlags::DOWNLOAD, FileFlags::EXTRACT_ARCHIVE],
            pad if version >= 6.7 => 8,
        )?;

        if file.bitness == Bitness::Bit32 {
            file.flags |= FileFlags::BITS_32;
        } else if file.bitness == Bitness::Bit64 {
            file.flags |= FileFlags::BITS_64;
        }

        file.r#type = FileType::try_read_from_io(&mut reader)?;

        Ok(file)
    }

    /// Returns the parsed installation conditions without evaluating scripts.
    pub fn condition(&self) -> &Condition {
        &self.condition
    }

    /// Returns the source file path as a string slice.
    #[must_use]
    #[inline]
    pub fn source(&self) -> Option<&str> {
        self.source.as_deref()
    }

    /// Returns the destination file path as a string slice.
    #[must_use]
    #[inline]
    pub fn destination(&self) -> Option<&str> {
        self.destination.as_deref()
    }

    /// Returns the destination file path as an owned [`String`], with any `{...}\`
    /// prefix removed, and backslashes normalized to forward slashes.
    ///
    /// # Examples
    ///
    /// - `{app}\bin\foo.dll` -> `bin/foo.dll`
    /// - `{sys}\bar.dll` -> `bar.dll`
    /// - `plain\path.txt` -> `plain/path.txt`
    pub fn normalized_destination(&self) -> Option<String> {
        let mut destination = self.destination()?;

        if destination.starts_with('{') {
            // Strip the `{...}\` or `{.../` prefix
            destination = destination.find('}').map_or(destination, |index| {
                let rest = &destination[index + 1..];
                rest.strip_prefix('\\')
                    .or_else(|| rest.strip_prefix('/'))
                    .unwrap_or(rest)
            })
        }

        Some(destination.replace('\\', "/"))
    }

    /// Returns the install font name as a string slice.
    #[must_use]
    #[inline]
    pub fn install_font_name(&self) -> Option<&str> {
        self.install_font_name.as_deref()
    }

    /// Returns the strong assembly name as a string slice.
    #[must_use]
    #[inline]
    pub fn strong_assembly_name(&self) -> Option<&str> {
        self.strong_assembly_name.as_deref()
    }

    /// Returns the location index into the data entry list.
    #[must_use]
    #[inline]
    pub const fn location(&self) -> u32 {
        self.location
    }

    /// Returns the attributes of the file.
    #[must_use]
    #[inline]
    pub const fn attributes(&self) -> u32 {
        self.attributes
    }

    /// Returns the external size of the file.
    #[must_use]
    #[inline]
    pub const fn external_size(&self) -> u64 {
        self.external_size
    }

    /// Returns the permission index into the permission entry list.
    #[must_use]
    #[inline]
    pub const fn permission(&self) -> i16 {
        self.permission
    }

    /// Returns the bitness of the file.
    #[must_use]
    #[inline]
    pub const fn bitness(&self) -> Bitness {
        self.bitness
    }

    /// Returns the flags associated with the file.
    #[must_use]
    #[inline]
    pub const fn flags(&self) -> FileFlags {
        self.flags
    }

    /// Returns the type of the file.
    #[must_use]
    #[inline]
    pub const fn r#type(&self) -> FileType {
        self.r#type
    }
}

impl Default for File {
    fn default() -> Self {
        Self {
            source: None,
            destination: None,
            install_font_name: None,
            strong_assembly_name: None,
            excludes: None,
            download_is_sig_source: None,
            download_user_name: None,
            download_password: None,
            extract_archive_password: None,
            verification: None,
            condition: Condition::default(),
            location: 0,
            attributes: 0,
            external_size: 0,
            permission: -1,
            bitness: Bitness::default(),
            flags: FileFlags::default(),
            r#type: FileType::default(),
        }
    }
}

#[cfg(test)]
mod tests {
    use rstest::rstest;

    use super::File;

    #[rstest]
    #[case::app(r"{app}\bin\foo.dll", "bin/foo.dll")]
    #[case::sys(r"{sys}\bar.dll", "bar.dll")]
    #[case::plain(r"plain\path.txt", "plain/path.txt")]
    #[case::empty_after_prefix(r"{app}", "")]
    fn resolve_path(#[case] destination: &str, #[case] expected: &str) {
        let file = File {
            destination: Some(destination.into()),
            ..File::default()
        };

        assert_eq!(file.normalized_destination().as_deref(), Some(expected));
    }
}
