use std::io;

use zerocopy::LE;

use crate::{read::ReadBytesExt, version::InnoVersion};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct EntryCounts {
    language: u32,
    custom_message: u32,
    permission: u32,
    r#type: u32,
    component: u32,
    task: u32,
    directory: u32,
    is_sig_key: u32,
    file: u32,
    file_location: u32,
    icon: u32,
    ini: u32,
    registry: u32,
    install_delete: u32,
    uninstall_delete: u32,
    run: u32,
    uninstall_run: u32,
}

impl EntryCounts {
    pub fn read<R>(mut reader: R, version: InnoVersion) -> io::Result<Self>
    where
        R: io::Read,
    {
        let mut counts = Self::default();

        if version >= 4 {
            counts.language = reader.read_u32::<LE>()?;
        } else if version >= (2, 0, 1) {
            counts.language = 1;
        }

        if version >= (4, 2, 1) {
            counts.custom_message = reader.read_u32::<LE>()?;
        }

        if version >= 4.1 {
            counts.permission = reader.read_u32::<LE>()?;
        }

        if version >= 2 || version.is_isx() {
            counts.r#type = reader.read_u32::<LE>()?;
            counts.component = reader.read_u32::<LE>()?;
        }

        if version >= 2 || (version.is_isx() && version >= (1, 3, 17)) {
            counts.task = reader.read_u32::<LE>()?;
        }

        counts.directory = reader.read_u32::<LE>()?;

        if version >= 6.5 {
            counts.is_sig_key = reader.read_u32::<LE>()?;
        }

        counts.file = reader.read_u32::<LE>()?;
        counts.file_location = reader.read_u32::<LE>()?;
        counts.icon = reader.read_u32::<LE>()?;
        counts.ini = reader.read_u32::<LE>()?;
        counts.registry = reader.read_u32::<LE>()?;
        counts.install_delete = reader.read_u32::<LE>()?;
        counts.uninstall_delete = reader.read_u32::<LE>()?;
        counts.run = reader.read_u32::<LE>()?;
        counts.uninstall_run = reader.read_u32::<LE>()?;

        Ok(counts)
    }

    /// Returns the number of [Language] entries.
    ///
    /// [Language]: crate::entry::Language
    #[must_use]
    #[inline]
    pub const fn language(&self) -> u32 {
        self.language
    }

    /// Returns the number of [Custom Message] entries.
    ///
    /// [Custom Message]: crate::entry::MessageEntry
    #[must_use]
    #[inline]
    pub const fn custom_message(&self) -> u32 {
        self.custom_message
    }

    /// Returns the number of [Permission] entries.
    ///
    /// [Permission]: crate::entry::Permission
    #[must_use]
    #[inline]
    pub const fn permission(&self) -> u32 {
        self.permission
    }

    /// Returns the number of [Type] entries.
    ///
    /// [Type]: crate::entry::Type
    #[must_use]
    #[inline]
    pub const fn r#type(&self) -> u32 {
        self.r#type
    }

    /// Returns the number of [Component] entries.
    ///
    /// [Component]: crate::entry::Component
    #[must_use]
    #[inline]
    pub const fn component(&self) -> u32 {
        self.component
    }

    /// Returns the number of [Task] entries.
    ///
    /// [Task]: crate::entry::Task
    #[must_use]
    #[inline]
    pub const fn task(&self) -> u32 {
        self.task
    }

    /// Returns the number of [Directory] entries.
    ///
    /// [Directory]: crate::entry::Directory
    #[must_use]
    #[inline]
    pub const fn directory(&self) -> u32 {
        self.directory
    }

    /// Returns the number of [IS Sig Key] entries.
    ///
    /// [ISSigKey]: crate::entry::ISSigKey
    #[must_use]
    #[inline]
    pub const fn is_sig_key(&self) -> u32 {
        self.is_sig_key
    }

    /// Returns the number of [File] entries.
    ///
    /// [File]: crate::entry::File
    #[must_use]
    #[inline]
    pub const fn file(&self) -> u32 {
        self.file
    }

    /// Returns the number of [File Location] entries.
    ///
    /// [File Location]: crate::entry::FileLocation
    #[must_use]
    #[inline]
    pub const fn file_location(&self) -> u32 {
        self.file_location
    }

    /// Returns the number of [Icon] entries.
    ///
    /// [Icon]: crate::entry::Icon
    #[must_use]
    #[inline]
    pub const fn icon(&self) -> u32 {
        self.icon
    }

    /// Returns the number of [Ini] entries.
    ///
    /// [Ini]: crate::entry::Ini
    #[must_use]
    #[inline]
    pub const fn ini(&self) -> u32 {
        self.ini
    }

    /// Returns the number of [Registry] entries.
    ///
    /// [Registry]: crate::entry::RegistryEntry
    #[must_use]
    #[inline]
    pub const fn registry(&self) -> u32 {
        self.registry
    }

    /// Returns the number of [Install Delete] entries.
    ///
    /// [Install Delete]: crate::entry::DeleteEntry
    #[must_use]
    #[inline]
    pub const fn install_delete(&self) -> u32 {
        self.install_delete
    }

    /// Returns the number of [Uninstall Delete] entries.
    ///
    /// [Uninstall Delete]: crate::entry::DeleteEntry
    #[must_use]
    #[inline]
    pub const fn uninstall_delete(&self) -> u32 {
        self.uninstall_delete
    }

    /// Returns the number of [Run] entries.
    ///
    /// [Run]: crate::entry::RunEntry
    #[must_use]
    #[inline]
    pub const fn run(&self) -> u32 {
        self.run
    }

    /// Returns the number of [Uninstall Run] entries.
    ///
    /// [Uninstall Run]: crate::entry::RunEntry
    #[must_use]
    #[inline]
    pub const fn uninstall_run(&self) -> u32 {
        self.uninstall_run
    }
}
