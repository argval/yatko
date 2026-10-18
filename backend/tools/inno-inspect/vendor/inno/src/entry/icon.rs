use std::{fmt, io};

use bitflags::bitflags;
use encoding_rs::Encoding;
use zerocopy::{Immutable, KnownLayout, LE, TryFromBytes, ValidityError, try_transmute};

use super::Condition;
use crate::{
    InnoVersion, ReadBytesExt, WindowsVersionRange, header::flag_reader::read_flags::read_flags,
};

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Icon {
    name: Option<String>,
    filename: Option<String>,
    parameters: Option<String>,
    working_directory: Option<String>,
    file: Option<String>,
    comment: Option<String>,
    app_user_model_id: Option<String>,
    app_user_model_toast_activator_clsid: String,
    index: i32,
    show_command: i32,
    close_on_exit: CloseSetting,
    hotkey: u16,
    flags: IconFlags,
}

impl Icon {
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

        let mut icon = Self {
            name: reader.read_decoded_pascal_string(codepage)?,
            filename: reader.read_decoded_pascal_string(codepage)?,
            parameters: reader.read_decoded_pascal_string(codepage)?,
            working_directory: reader.read_decoded_pascal_string(codepage)?,
            file: reader.read_decoded_pascal_string(codepage)?,
            comment: reader.read_decoded_pascal_string(codepage)?,
            ..Self::default()
        };

        Condition::read(&mut reader, codepage, version)?;

        if version >= (5, 3, 5) {
            icon.app_user_model_id = reader.read_decoded_pascal_string(codepage)?;
        }

        if version >= 6.1 {
            let mut buf = [0; 16];
            reader.read_exact(&mut buf)?;
            icon.app_user_model_toast_activator_clsid = codepage.decode(&buf).0.into_owned();
        }

        WindowsVersionRange::read_from(&mut reader, version)?;

        icon.index = reader.read_i32::<LE>()?;

        if version >= (1, 3, 24) {
            icon.show_command = reader.read_i32::<LE>()?;
        }

        if version >= (1, 3, 15) {
            icon.close_on_exit = CloseSetting::try_read_from_io(&mut reader)?;
        }

        if version >= (2, 0, 7) {
            icon.hotkey = reader.read_u16::<LE>()?;
        }

        icon.flags = read_flags!(&mut reader,
            IconFlags::NEVER_UNINSTALL,
            if version < (1, 3, 26) => IconFlags::RUN_MINIMIZED,
            [IconFlags::CREATE_ONLY_IF_FILE_EXISTS, IconFlags::USE_APP_PATHS],
            if ((5, 0, 3)..(6, 3, 0)).contains(&version) => IconFlags::FOLDER_SHORTCUT,
            if version >= (5, 4, 2) => IconFlags::EXCLUDE_FROM_SHOW_IN_NEW_INSTALL,
            if version >= 5.5 => IconFlags::PREVENT_PINNING,
            if version >= 6.1 => IconFlags::HAS_APP_USER_MODEL_TOAST_ACTIVATOR_CLSID
        )?;

        Ok(icon)
    }

    /// Returns the name of the icon as a string slice.
    #[must_use]
    #[inline]
    pub fn name(&self) -> Option<&str> {
        self.name.as_deref()
    }

    /// Returns the filename of the icon as a string slice.
    #[must_use]
    #[inline]
    pub fn filename(&self) -> Option<&str> {
        self.filename.as_deref()
    }

    /// Returns the parameters of the icon as a string slice.
    #[must_use]
    #[inline]
    pub fn parameters(&self) -> Option<&str> {
        self.parameters.as_deref()
    }

    /// Returns the working directory of the icon as a string slice.
    #[must_use]
    #[inline]
    pub fn working_directory(&self) -> Option<&str> {
        self.working_directory.as_deref()
    }

    /// Returns the file of the icon as a string slice.
    #[must_use]
    #[inline]
    pub fn file(&self) -> Option<&str> {
        self.file.as_deref()
    }

    /// Returns the comment of the icon as a string slice.
    #[must_use]
    #[inline]
    pub fn comment(&self) -> Option<&str> {
        self.comment.as_deref()
    }

    /// Returns the `AppUserModelId` of the icon as a string slice.
    #[must_use]
    #[inline]
    pub fn app_user_model_id(&self) -> Option<&str> {
        self.app_user_model_id.as_deref()
    }

    /// Returns the `AppUserModelToastActivatorClsid` of the icon as a string slice.
    #[must_use]
    #[inline]
    pub const fn app_user_model_toast_activator_clsid(&self) -> &str {
        self.app_user_model_toast_activator_clsid.as_str()
    }

    /// Returns the index of the icon.
    #[must_use]
    #[inline]
    pub const fn index(&self) -> i32 {
        self.index
    }

    /// Returns the show command of the icon.
    #[must_use]
    #[inline]
    pub const fn show_command(&self) -> i32 {
        self.show_command
    }

    /// Returns the close on exit setting of the icon.
    #[must_use]
    #[inline]
    pub const fn close_on_exit(&self) -> CloseSetting {
        self.close_on_exit
    }

    /// Returns the hotkey of the icon.
    #[must_use]
    #[inline]
    pub const fn hotkey(&self) -> u16 {
        self.hotkey
    }

    /// Returns the flags of the icon.
    #[must_use]
    #[inline]
    pub const fn flags(&self) -> IconFlags {
        self.flags
    }
}

impl Default for Icon {
    fn default() -> Self {
        Self {
            name: None,
            filename: None,
            parameters: None,
            working_directory: None,
            file: None,
            comment: None,
            app_user_model_id: None,
            app_user_model_toast_activator_clsid: String::new(),
            index: 0,
            show_command: 1,
            close_on_exit: CloseSetting::default(),
            hotkey: 0,
            flags: IconFlags::default(),
        }
    }
}

/// <https://github.com/jrsoftware/issrc/blob/is-6_5_1/Projects/Src/Shared.Struct.pas#L291>
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum CloseSetting {
    #[default]
    NoSetting = 0,
    CloseOnExit = 1,
    DontCloseOnExit = 2,
}

impl CloseSetting {
    pub fn try_read_from_io<R>(mut src: R) -> io::Result<Self>
    where
        Self: Sized,
        R: io::Read,
    {
        src.read_u8().and_then(|value| {
            Self::try_from(value).map_err(|err| io::Error::new(io::ErrorKind::InvalidData, err))
        })
    }

    /// Returns the Close Setting as a static string.
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::NoSetting => "No setting",
            Self::CloseOnExit => "Close on exit",
            Self::DontCloseOnExit => "Dont close on exit",
        }
    }
}

impl fmt::Display for CloseSetting {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}

impl TryFrom<u8> for CloseSetting {
    type Error = ValidityError<u8, Self>;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        try_transmute!(value)
    }
}

bitflags! {
    #[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
    pub struct IconFlags: u8 {
        const NEVER_UNINSTALL = 1;
        const CREATE_ONLY_IF_FILE_EXISTS = 1 << 1;
        const USE_APP_PATHS = 1 << 2;
        const FOLDER_SHORTCUT = 1 << 3;
        const EXCLUDE_FROM_SHOW_IN_NEW_INSTALL = 1 << 4;
        const PREVENT_PINNING = 1 << 5;
        const HAS_APP_USER_MODEL_TOAST_ACTIVATOR_CLSID = 1 << 6;
        const RUN_MINIMIZED = 1 << 7;
    }
}

impl fmt::Display for IconFlags {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        bitflags::parser::to_writer(self, f)
    }
}
