mod architecture;
mod auto_bool;
mod compression;
mod entry_counts;
pub mod flag_reader;
mod flags;
mod install_verbosity;
mod language_detection;
mod log_mode;
mod privilege_level;
mod privileges_required_overrides;
mod wizard;
mod yes_no;

use std::{fmt, io};

pub use architecture::{Architecture, StoredArchitecture};
pub use auto_bool::AutoBool;
pub use compression::Compression;
use encoding_rs::{Encoding, WINDOWS_1252};
pub use entry_counts::EntryCounts;
use flag_reader::read_flags::read_flags;
pub use flags::HeaderFlags;
pub use install_verbosity::InstallVerbosity;
pub use language_detection::LanguageDetection;
pub use log_mode::LogMode;
pub use privilege_level::PrivilegeLevel;
pub use privileges_required_overrides::PrivilegesRequiredOverrides;
pub use wizard::{Color, ImageAlphaFormat, WizardSettings, WizardSizePercent, WizardStyle};
use yes_no::YesNoStr;
use zerocopy::LE;

use super::{InnoVersion, WindowsVersionRange, read::ReadBytesExt};
use crate::{
    encryption::EncryptionHeader,
    entry::Checksum,
    error::{InnoError, InnoResult},
    header::wizard::LightControlStyling,
    slice::SlicesPerDisk,
    string::PascalString,
};

// https://github.com/jrsoftware/issrc/blob/main/Projects/Src/Shared.Struct.pas
#[derive(Clone, Default, Eq, PartialEq)]
pub struct Header {
    pub flags: HeaderFlags,
    #[doc(alias = "AppName")]
    app_name: Option<PascalString>,
    #[doc(alias = "AppVerName")]
    app_versioned_name: Option<PascalString>,
    /// <https://jrsoftware.org/ishelp/index.php?topic=setup_appid>
    app_id: Option<PascalString>,
    app_copyright: Option<PascalString>,
    app_publisher: Option<PascalString>,
    app_publisher_url: Option<PascalString>,
    app_support_phone: Option<PascalString>,
    app_support_url: Option<PascalString>,
    app_updates_url: Option<PascalString>,
    app_version: Option<PascalString>,
    default_dir_name: Option<PascalString>,
    default_group_name: Option<PascalString>,
    uninstall_icon_name: Option<String>,
    base_filename: Option<PascalString>,
    uninstall_files_dir: Option<PascalString>,
    uninstall_name: Option<PascalString>,
    uninstall_icon: Option<PascalString>,
    app_mutex: Option<PascalString>,
    default_user_name: Option<PascalString>,
    default_user_organisation: Option<PascalString>,
    default_serial: Option<PascalString>,
    app_readme_file: Option<PascalString>,
    app_contact: Option<PascalString>,
    app_comments: Option<PascalString>,
    app_modify_path: Option<PascalString>,
    create_uninstall_registry_key: Option<PascalString>,
    uninstallable: Option<PascalString>,
    close_applications_filter: Option<PascalString>,
    setup_mutex: Option<PascalString>,
    changes_environment: Option<PascalString>,
    changes_associations: Option<PascalString>,
    architectures_allowed_expr: Option<PascalString>,
    architectures_allowed: Architecture,
    architectures_disallowed: Architecture,
    architectures_install_in_64_bit_mode: Architecture,
    architectures_install_in_64_bit_mode_expr: Option<PascalString>,
    close_applications_filter_excludes: Option<PascalString>,
    seven_zip_library_name: Option<PascalString>,
    use_previous_app_dir: Option<PascalString>,
    use_previous_group: Option<PascalString>,
    use_previous_setup_type: Option<PascalString>,
    use_previous_tasks: Option<PascalString>,
    use_previous_user_info: Option<PascalString>,
    license_text: Option<String>,
    info_before: Option<String>,
    info_after: Option<String>,
    uninstaller_signature: Option<String>,
    compiled_code: Option<String>,
    lead_bytes: [u8; 256 / u8::BITS as usize],
    entry_counts: EntryCounts,
    compiled_code_version: Option<u32>,
    background_color: Color,
    background_color2: Color,
    wizard: WizardSettings,
    encryption_header: Option<EncryptionHeader>,
    extra_disk_space_required: u64,
    slices_per_disk: SlicesPerDisk,
    install_verbosity: InstallVerbosity,
    uninstall_log_mode: LogMode,
    uninstall_style: WizardStyle,
    dir_exists_warning: AutoBool,
    privileges_required: PrivilegeLevel,
    privileges_required_overrides_allowed: PrivilegesRequiredOverrides,
    show_language_dialog: AutoBool,
    language_detection: LanguageDetection,
    compression: Compression,
    signed_uninstaller_original_size: u32,
    signed_uninstaller_header_checksum: u32,
    disable_dir_page: AutoBool,
    disable_program_group_page: AutoBool,
    uninstall_display_size: u64,
}

impl Header {
    pub fn read<R>(mut reader: R, version: InnoVersion) -> InnoResult<Self>
    where
        R: io::Read,
    {
        let mut header = Self::default();

        if version < 1.3 {
            let _uncompressed_size = reader.read_u32::<LE>()?;
        }

        header.app_name = reader.read_pascal_string()?;
        header.app_versioned_name = reader.read_pascal_string()?;
        if version >= 1.3 {
            header.app_id = reader.read_pascal_string()?;
        }
        header.app_copyright = reader.read_pascal_string()?;
        if version >= 1.3 {
            header.app_publisher = reader.read_pascal_string()?;
            header.app_publisher_url = reader.read_pascal_string()?;
        }
        if version >= (5, 1, 13) {
            header.app_support_phone = reader.read_pascal_string()?;
        }
        if version >= 1.3 {
            header.app_support_url = reader.read_pascal_string()?;
            header.app_updates_url = reader.read_pascal_string()?;
            header.app_version = reader.read_pascal_string()?;
        }
        header.default_dir_name = reader.read_pascal_string()?;
        header.default_group_name = reader.read_pascal_string()?;
        if version < 3 {
            header.uninstall_icon_name = reader.read_decoded_pascal_string(WINDOWS_1252)?;
        }
        header.base_filename = reader.read_pascal_string()?;
        if ((1, 3, 0)..(5, 2, 5)).contains(&version) {
            header.license_text = reader.read_decoded_pascal_string(WINDOWS_1252)?;
            header.info_before = reader.read_decoded_pascal_string(WINDOWS_1252)?;
            header.info_after = reader.read_decoded_pascal_string(WINDOWS_1252)?;
        }
        if version >= (1, 3, 3) {
            header.uninstall_files_dir = reader.read_pascal_string()?;
        }
        if version >= (1, 3, 6) {
            header.uninstall_name = reader.read_pascal_string()?;
            header.uninstall_icon = reader.read_pascal_string()?;
        }
        if version >= (1, 3, 14) {
            header.app_mutex = reader.read_pascal_string()?;
        }
        if version >= 3 {
            header.default_user_name = reader.read_pascal_string()?;
            header.default_user_organisation = reader.read_pascal_string()?;
        }
        if version >= 4 || (version.is_isx() && version >= (3, 0, 6, 1)) {
            header.default_serial = reader.read_pascal_string()?;
        }
        if ((4, 0, 0)..(5, 2, 5)).contains(&version) || (version.is_isx() && version >= (1, 3, 24))
        {
            header.compiled_code = reader.read_decoded_pascal_string(WINDOWS_1252)?;
        }
        if version >= (4, 2, 4) {
            header.app_readme_file = reader.read_pascal_string()?;
            header.app_contact = reader.read_pascal_string()?;
            header.app_comments = reader.read_pascal_string()?;
            header.app_modify_path = reader.read_pascal_string()?;
        }
        if version >= (5, 3, 8) {
            header.create_uninstall_registry_key = reader.read_pascal_string()?;
        }
        if version >= (5, 3, 10) {
            header.uninstallable = reader.read_pascal_string()?;
        }
        if version >= 5.5 {
            header.close_applications_filter = reader.read_pascal_string()?;
        }
        if version >= (5, 5, 6) {
            header.setup_mutex = reader.read_pascal_string()?;
        }
        if version >= (5, 6, 1) {
            header.changes_environment = reader.read_pascal_string()?;
            header.changes_associations = reader.read_pascal_string()?;
        }
        if version >= 6.3 {
            header.architectures_allowed_expr = reader.read_pascal_string()?;
            header.architectures_install_in_64_bit_mode_expr = reader.read_pascal_string()?;
        }
        if version >= (6, 4, 2) {
            header.close_applications_filter_excludes = reader.read_pascal_string()?;
        }
        if version >= 6.5 {
            header.seven_zip_library_name = reader.read_pascal_string()?;
        }
        if version >= 6.7 {
            header.use_previous_app_dir = reader.read_pascal_string()?;
            header.use_previous_group = reader.read_pascal_string()?;
            header.use_previous_setup_type = reader.read_pascal_string()?;
            header.use_previous_tasks = reader.read_pascal_string()?;
            header.use_previous_user_info = reader.read_pascal_string()?;
        }
        if version >= (5, 2, 5) {
            header.license_text = reader.read_decoded_pascal_string(WINDOWS_1252)?;
            header.info_before = reader.read_decoded_pascal_string(WINDOWS_1252)?;
            header.info_after = reader.read_decoded_pascal_string(WINDOWS_1252)?;
        }
        if ((5, 2, 1)..(5, 3, 10)).contains(&version) {
            header.uninstaller_signature = reader.read_decoded_pascal_string(WINDOWS_1252)?;
        }
        if version >= (5, 2, 5) {
            header.compiled_code = reader.read_decoded_pascal_string(WINDOWS_1252)?;
        }
        if version >= (2, 0, 6) && !version.is_unicode() {
            let mut buf = [0; 256 / u8::BITS as usize];
            reader.read_exact(&mut buf)?;
            header.lead_bytes = buf;
        }
        header.entry_counts = EntryCounts::read(&mut reader, version)?;
        if version >= (7, 0, 0, 3) {
            header.compiled_code_version = Some(reader.read_u32::<LE>()?);
        }
        let license_size = if version < 1.3 {
            reader.read_u32::<LE>()?
        } else {
            0
        };
        let info_before_size = if version < 1.3 {
            reader.read_u32::<LE>()?
        } else {
            0
        };
        let info_after_size = if version < 1.3 {
            reader.read_u32::<LE>()?
        } else {
            0
        };
        WindowsVersionRange::read_from(&mut reader, version)?;
        if version < (6, 4, 0, 1) {
            header.background_color = reader.read_t::<Color>()?;
        }
        if version >= (1, 3, 3) && version < (6, 4, 0, 1) {
            header.background_color2 = reader.read_t::<Color>()?;
        }
        header.wizard = WizardSettings::read_from(&mut reader, version)?;
        if (6.4..6.5).contains(&version) {
            header.encryption_header = Some(EncryptionHeader::read(&mut reader, version)?);
        } else if version < 6.4 {
            let _password_hash = if version >= (5, 3, 9) {
                Checksum::read_sha1(&mut reader)?
            } else if version >= 4.2 {
                Checksum::read_md5(&mut reader)?
            } else {
                Checksum::read_crc32(&mut reader)?
            };
            if version >= (4, 2, 2) {
                let _password_salt = {
                    let mut password_salt_buf = [0; 8];
                    reader.read_exact(&mut password_salt_buf)?;
                    password_salt_buf
                };
            }
        }

        if version >= (6, 5, 2) {
            header.wizard.image_back_color = reader.read_t::<Color>()?;
            header.wizard.small_image_back_color = reader.read_t::<Color>()?;

            if version >= 6.7 {
                header.wizard.back_color = reader.read_t::<Color>()?;
            }
        }

        if version >= 6.6 {
            header.wizard.image_back_color_dynamic_dark = reader.read_t::<Color>()?;
            header.wizard.small_image_back_color_dynamic_dark = reader.read_t::<Color>()?;

            if version >= 6.7 {
                header.wizard.back_color_dynamic_dark = reader.read_t::<Color>()?;
            }
        }

        if version >= (6, 6, 1) {
            header.wizard.image_opacity = Some(reader.read_u8()?);

            if version >= 6.7 {
                header.wizard.back_image_opacity = Some(reader.read_u8()?);
                header.wizard.light_control_setting =
                    Some(LightControlStyling::try_read_from_io(&mut reader)?);
            }
        }

        if version >= 4 {
            header.extra_disk_space_required = reader.read_u64::<LE>()?;
            let slices_per_disk = reader.read_u32::<LE>()?;
            header.slices_per_disk = slices_per_disk
                .try_into()
                .map_err(InnoError::InvalidSlicesPerDisk)?;
        } else {
            header.extra_disk_space_required = u64::from(reader.read_u32::<LE>()?);
            header.slices_per_disk = SlicesPerDisk::default();
        }
        if (2..5).contains(&version) || (version.is_isx() && version >= (1, 3, 4)) {
            header.install_verbosity = InstallVerbosity::try_read_from_io(&mut reader)?;
        }
        if version >= 1.3 {
            header.uninstall_log_mode = LogMode::try_read_from_io(&mut reader)?;
        }
        if version >= 5 {
            header.uninstall_style = WizardStyle::Modern;
        } else if version >= 2 || (version.is_isx() && version >= (1, 3, 13)) {
            header.uninstall_style = WizardStyle::try_read_from(&mut reader, version)?;
        }
        if version >= (1, 3, 6) {
            header.dir_exists_warning = AutoBool::try_read_from_io(&mut reader)?;
        }
        if version.is_isx() && ((2, 0, 10)..(3, 0, 0)).contains(&version) {
            let _code_line_offset = reader.read_u32::<LE>()?;
        }
        if ((3, 0, 0)..(3, 0, 3)).contains(&version) {
            match AutoBool::try_read_from_io(&mut reader) {
                Ok(AutoBool::Yes) => header.flags |= HeaderFlags::ALWAYS_RESTART,
                Ok(AutoBool::Auto) => {
                    header.flags |= HeaderFlags::RESTART_IF_NEEDED_BY_RUN;
                }
                _ => {}
            }
        }
        if version >= (3, 0, 4) || (version.is_isx() && version >= (3, 0, 3)) {
            header.privileges_required = PrivilegeLevel::try_read_from_io(&mut reader)?;
        }
        if version >= 5.7 {
            header.privileges_required_overrides_allowed =
                PrivilegesRequiredOverrides::from_bits_retain(reader.read_u8()?);
        }
        if version >= (4, 0, 10) {
            header.show_language_dialog = AutoBool::try_read_from_io(&mut reader)?;
            header.language_detection = LanguageDetection::try_read_from_io(&mut reader)?;
        }
        if version >= (4, 2, 6) {
            header.compression = Compression::try_read_from_io(&mut reader)?;
        } else if version == (4, 2, 5) {
            header.compression = match reader.read_u8()? {
                0 => Compression::Stored,
                1 => Compression::BZip2,
                2 => Compression::LZMA1,
                _ => Compression::Unknown,
            };
        } else if version >= (4, 1, 5) {
            header.compression = match reader.read_u8()? {
                0 => Compression::Zlib,
                1 => Compression::BZip2,
                _ => Compression::Unknown,
            };
        }
        if ((5, 1)..(6, 3)).contains(&version) {
            header.architectures_allowed =
                StoredArchitecture::from_bits_retain(reader.read_u8()?).into();
            header.architectures_install_in_64_bit_mode =
                StoredArchitecture::from_bits_retain(reader.read_u8()?).into();
        } else if version < 5.1 {
            header.architectures_allowed = StoredArchitecture::all().into();
            header.architectures_install_in_64_bit_mode = StoredArchitecture::all().into();
        }
        if ((5, 2, 1)..(5, 3, 10)).contains(&version) {
            header.signed_uninstaller_original_size = reader.read_u32::<LE>()?;
            header.signed_uninstaller_header_checksum = reader.read_u32::<LE>()?;
        }
        if version >= (5, 3, 3) {
            header.disable_dir_page = AutoBool::try_read_from_io(&mut reader)?;
            header.disable_program_group_page = AutoBool::try_read_from_io(&mut reader)?;
        }
        if version >= 5.5 {
            header.uninstall_display_size = reader.read_u64::<LE>()?;
        } else if version >= (5, 3, 6) {
            header.uninstall_display_size = u64::from(reader.read_u32::<LE>()?);
        }

        if version.is_blackbox() {
            reader.read_u8()?;
        }

        header.flags |= Self::read_flags(&mut reader, version)?;

        if version < (3, 0, 4) {
            header.privileges_required = PrivilegeLevel::from(header.flags);
        }
        if version < (4, 0, 10) {
            header.show_language_dialog =
                AutoBool::from_header_flags(&header.flags, HeaderFlags::SHOW_LANGUAGE_DIALOG);
            header.language_detection = LanguageDetection::from(header.flags);
        }
        if version < (4, 1, 5) {
            header.compression = Compression::from(header.flags);
        }
        if version < (5, 3, 3) {
            header.disable_dir_page =
                AutoBool::from_header_flags(&header.flags, HeaderFlags::DISABLE_DIR_PAGE);
            header.disable_program_group_page =
                AutoBool::from_header_flags(&header.flags, HeaderFlags::DISABLE_PROGRAM_GROUP_PAGE);
        }
        if version < 1.3 {
            header.license_text =
                reader.read_sized_decoded_pascal_string(license_size, WINDOWS_1252)?;
            header.info_before =
                reader.read_sized_decoded_pascal_string(info_before_size, WINDOWS_1252)?;
            header.info_after =
                reader.read_sized_decoded_pascal_string(info_after_size, WINDOWS_1252)?;
        }

        Ok(header)
    }

    fn read_flags<R>(reader: &mut R, version: InnoVersion) -> io::Result<HeaderFlags>
    where
        R: io::Read,
    {
        read_flags!(reader,
            HeaderFlags::DISABLE_STARTUP_PROMPT,
            if version < (5, 3, 10) => HeaderFlags::UNINSTALLABLE,
            HeaderFlags::CREATE_APP_DIR,
            if version < (5, 3, 3) => HeaderFlags::DISABLE_DIR_PAGE,
            if version < (1, 3, 6) => HeaderFlags::DISABLE_DIR_EXISTS_WARNING,
            if version < (5, 3, 3) => HeaderFlags::DISABLE_PROGRAM_GROUP_PAGE,
            HeaderFlags::ALLOW_NO_ICONS,
            if !((3, 0, 0)..(3, 0, 3)).contains(&version) => HeaderFlags::ALWAYS_RESTART,
            if version < (1, 3, 3) => HeaderFlags::BACK_SOLID,
            HeaderFlags::ALWAYS_USE_PERSONAL_GROUP,
            if version < (6, 4, 0, 1) => [
                HeaderFlags::WINDOW_VISIBLE,
                HeaderFlags::WINDOW_SHOW_CAPTION,
                HeaderFlags::WINDOW_RESIZABLE,
                HeaderFlags::WINDOW_START_MAXIMISED,
            ],
            HeaderFlags::ENABLED_DIR_DOESNT_EXIST_WARNING,
            if version < (4, 1, 2) => HeaderFlags::DISABLE_APPEND_DIR,
            HeaderFlags::PASSWORD,
            if version >= (1, 2, 6) => HeaderFlags::ALLOW_ROOT_DIRECTORY,
            if version >= (1, 2, 14) => HeaderFlags::DISABLE_FINISHED_PAGE,
            if version < (3, 0, 4) => HeaderFlags::ADMIN_PRIVILEGES_REQUIRED,
            if version < 3 => HeaderFlags::ALWAYS_CREATE_UNINSTALL_ICON,
            if version < (1, 3, 6) => HeaderFlags::OVERWRITE_UNINSTALL_REG_ENTRIES,
            if version < (5, 6, 1) => HeaderFlags::CHANGES_ASSOCIATIONS,
            if ((1, 3, 0)..(5, 3, 8)).contains(&version) => HeaderFlags::CREATE_UNINSTALL_REG_KEY,
            if ((1, 3, 1)..(6, 7, 0)).contains(&version) => HeaderFlags::USE_PREVIOUS_APP_DIR,
            if version >= (1, 3, 3) && version < (6, 4, 0, 1) => HeaderFlags::BACK_COLOR_HORIZONTAL,
            if ((1, 3, 10)..(6, 7, 0)).contains(&version) => HeaderFlags::USE_PREVIOUS_GROUP,
            if version >= (1, 3, 20) => HeaderFlags::UPDATE_UNINSTALL_LOG_APP_NAME,
            if (version >= 2 || (version.is_isx() && version >= (1, 3, 10))) && version < 6.7 => HeaderFlags::USE_PREVIOUS_SETUP_TYPE,
            if version >= 2 => [
                HeaderFlags::DISABLE_READY_MEMO,
                HeaderFlags::ALWAYS_SHOW_COMPONENTS_LIST,
                HeaderFlags::FLAT_COMPONENTS_LIST,
                HeaderFlags::SHOW_COMPONENT_SIZES,
            ],
            if (2.0..6.7).contains(&version) => HeaderFlags::USE_PREVIOUS_TASKS,
            if version >= 2 => HeaderFlags::DISABLE_READY_PAGE,
            if version >= (2, 0, 7) => [
                HeaderFlags::ALWAYS_SHOW_DIR_ON_READY_PAGE,
                HeaderFlags::ALWAYS_SHOW_GROUP_ON_READY_PAGE,
            ],
            if ((2, 0, 17)..(4, 1, 5)).contains(&version) => HeaderFlags::BZIP_USED,
            if version >= (2, 0, 18) => HeaderFlags::ALLOW_UNC_PATH,
            if version >= 3 => HeaderFlags::USER_INFO_PAGE,
            if (3.0..6.7).contains(&version) => HeaderFlags::USE_PREVIOUS_USER_INFO,
            if version >= (3, 0, 1) => HeaderFlags::UNINSTALL_RESTART_COMPUTER,
            if version >= (3, 0, 3) => HeaderFlags::RESTART_IF_NEEDED_BY_RUN,
            if version >= 4 || (version.is_isx() && version >= (3, 0, 3)) => HeaderFlags::SHOW_TASKS_TREE_LINES,
            if ((4, 0, 1)..(4, 0, 10)).contains(&version) => HeaderFlags::DETECT_LANGUAGE_USING_LOCALE,
            if version >= (4, 0, 9) => HeaderFlags::ALLOW_CANCEL_DURING_INSTALL,
            if version >= (4, 1, 3) => HeaderFlags::WIZARD_IMAGE_STRETCH,
            if version >= (4, 1, 8) => [
                HeaderFlags::APPEND_DEFAULT_DIR_NAME,
                HeaderFlags::APPEND_DEFAULT_GROUP_NAME,
            ],
            if ((4, 2, 2)..(6, 5, 0)).contains(&version) => HeaderFlags::ENCRYPTION_USED,
            if ((5, 0, 4)..(5, 6, 1)).contains(&version) => HeaderFlags::CHANGES_ENVIRONMENT,
            if version >= (5, 1, 7) && !version.is_unicode() => HeaderFlags::SHOW_UNDISPLAYABLE_LANGUAGES,
            if version >= (5, 1, 13) => HeaderFlags::SETUP_LOGGING,
            if version >= (5, 2, 1) => HeaderFlags::SIGNED_UNINSTALLER,
            if version >= (5, 3, 8) => HeaderFlags::USE_PREVIOUS_LANGUAGE,
            if version >= (5, 3, 9) => HeaderFlags::DISABLE_WELCOME_PAGE,
            if version >= (5, 5, 0) => [
                HeaderFlags::CLOSE_APPLICATIONS,
                HeaderFlags::RESTART_APPLICATIONS,
                HeaderFlags::ALLOW_NETWORK_DRIVE,
            ],
            if version >= (5, 5, 7) => HeaderFlags::FORCE_CLOSE_APPLICATIONS,
            if version >= 6 => [
                HeaderFlags::APP_NAME_HAS_CONSTS,
                HeaderFlags::USE_PREVIOUS_PRIVILEGES,
            ],
            if (6.0..6.6).contains(&version) => HeaderFlags::WIZARD_RESIZABLE,
            if version >= 6.3 => HeaderFlags::UNINSTALL_LOGGING,
            if version >= 6.6 => [
                HeaderFlags::WIZARD_MODERN,
                HeaderFlags::WIZARD_BORDER_STYLED,
                HeaderFlags::WIZARD_KEEP_ASPECT_RATIO,
            ],
            if (6.6..6.7).contains(&version) => HeaderFlags::WIZARD_LIGHT_BUTTONS_UNSTYLED,
            if version >= 6.7 => [
                HeaderFlags::REDIRECTION_GUARD,
                HeaderFlags::WIZARD_BEVELS_HIDDEN,
            ],
            pad if version >= 6.7 => 8,
        ).map(|mut read_flags| {
            if version < (4, 0, 9) {
                read_flags |= HeaderFlags::ALLOW_CANCEL_DURING_INSTALL;
            }
            if version < 5.5 {
                read_flags |= HeaderFlags::ALLOW_NETWORK_DRIVE;
            }
            read_flags
        })
    }

    pub fn decode(&mut self, codepage: &'static Encoding) {
        macro_rules! decode {
            ( $( $field:ident ),* $(,)? ) => {
                $(
                    if let Some(ref mut $field) = self.$field {
                        $field.decode(codepage);
                    }
                )*
            };
        }

        decode!(
            app_name,
            app_versioned_name,
            app_id,
            app_copyright,
            app_publisher,
            app_publisher_url,
            app_support_phone,
            app_support_url,
            app_updates_url,
            app_version,
            default_dir_name,
            default_group_name,
            base_filename,
            uninstall_files_dir,
            uninstall_name,
            uninstall_icon,
            app_mutex,
            default_user_name,
            default_user_organisation,
            default_serial,
            app_readme_file,
            app_contact,
            app_comments,
            app_modify_path,
            create_uninstall_registry_key,
            uninstallable,
            close_applications_filter,
            setup_mutex,
            changes_environment,
            changes_associations,
            architectures_allowed_expr,
            architectures_install_in_64_bit_mode_expr,
            close_applications_filter_excludes,
            seven_zip_library_name,
            use_previous_app_dir,
            use_previous_group,
            use_previous_setup_type,
            use_previous_tasks,
            use_previous_user_info,
        );
    }

    /// Returns the Product Code as it would appear in Windows Registry.
    #[must_use]
    pub fn product_code(&self) -> Option<String> {
        /// Inno tags '_is1' onto the end of the App ID to create the Uninstall registry key
        const IS1_SUFFIX: &str = "_is1";

        let app_id = self.app_id()?;

        let product_code = if app_id.starts_with("{{") {
            &app_id[1..]
        } else {
            app_id
        };

        Some(format!("{product_code}{IS1_SUFFIX}"))
    }

    /// Returns the name of the application.
    #[doc(alias = "AppName")]
    #[must_use]
    pub fn app_name(&self) -> Option<&str> {
        self.app_name.as_ref().map(PascalString::as_str)
    }

    /// Returns the versioned name of the application.
    #[doc(alias = "AppVerName")]
    #[must_use]
    pub fn app_versioned_name(&self) -> Option<&str> {
        self.app_versioned_name.as_ref().map(PascalString::as_str)
    }

    /// Returns the ID of the application.
    ///
    /// See [AppId](https://jrsoftware.org/ishelp/index.php?topic=setup_appid).
    #[doc(alias = "AppId")]
    #[must_use]
    pub fn app_id(&self) -> Option<&str> {
        self.app_id.as_ref().map(PascalString::as_str)
    }

    /// Returns the copyright of the application.
    #[doc(alias = "AppCopyright")]
    #[must_use]
    pub fn app_copyright(&self) -> Option<&str> {
        self.app_copyright.as_ref().map(PascalString::as_str)
    }

    /// Returns the publisher of the application.
    #[doc(alias = "AppPublisher")]
    #[must_use]
    pub fn app_publisher(&self) -> Option<&str> {
        self.app_publisher.as_ref().map(PascalString::as_str)
    }

    /// Returns the URL of the publisher of the application.
    #[doc(alias = "AppPublisherURL")]
    #[must_use]
    pub fn app_publisher_url(&self) -> Option<&str> {
        self.app_publisher_url.as_ref().map(PascalString::as_str)
    }

    /// Returns the support phone number for the application.
    #[doc(alias = "AppSupportPhone")]
    #[must_use]
    pub fn app_support_phone(&self) -> Option<&str> {
        self.app_support_phone.as_ref().map(PascalString::as_str)
    }

    /// Returns the support URL for the application.
    #[doc(alias = "AppSupportURL")]
    #[must_use]
    pub fn app_support_url(&self) -> Option<&str> {
        self.app_support_url.as_ref().map(PascalString::as_str)
    }

    /// Returns the updates URL for the application.
    #[doc(alias = "AppUpdatesURL")]
    #[must_use]
    pub fn app_updates_url(&self) -> Option<&str> {
        self.app_updates_url.as_ref().map(PascalString::as_str)
    }

    /// Returns the version of the application.
    #[doc(alias = "AppVersion")]
    #[must_use]
    pub fn app_version(&self) -> Option<&str> {
        self.app_version.as_ref().map(PascalString::as_str)
    }

    /// Returns the default directory name for the application.
    #[doc(alias = "DefaultDirName")]
    #[must_use]
    pub fn default_dir_name(&self) -> Option<&str> {
        self.default_dir_name.as_ref().map(PascalString::as_str)
    }

    /// Returns the default group name for the application.
    #[doc(alias = "DefaultGroupName")]
    #[must_use]
    pub fn default_group_name(&self) -> Option<&str> {
        self.default_group_name.as_ref().map(PascalString::as_str)
    }

    /// Returns the uninstallation icon name for the application.
    #[doc(alias = "UninstallIconName")]
    #[must_use]
    pub fn uninstall_icon_name(&self) -> Option<&str> {
        self.uninstall_icon_name.as_deref()
    }

    /// Returns the base filename for the application.
    #[doc(alias = "BaseFileName")]
    #[must_use]
    pub fn base_filename(&self) -> Option<&str> {
        self.base_filename.as_ref().map(PascalString::as_str)
    }

    /// Returns the uninstallation files directory for the application.
    #[doc(alias = "UninstallFilesDir")]
    #[must_use]
    pub fn uninstall_files_dir(&self) -> Option<&str> {
        self.uninstall_files_dir.as_ref().map(PascalString::as_str)
    }

    /// Returns the uninstaller name for the application.
    #[doc(alias = "UninstallName")]
    #[must_use]
    pub fn uninstall_name(&self) -> Option<&str> {
        self.uninstall_name.as_ref().map(PascalString::as_str)
    }

    /// Returns the uninstaller icon for the application.
    #[doc(alias = "UninstallIcon")]
    #[must_use]
    pub fn uninstall_icon(&self) -> Option<&str> {
        self.uninstall_icon.as_ref().map(PascalString::as_str)
    }

    /// Returns the application mutex name.
    #[doc(alias = "AppMutex")]
    #[must_use]
    pub fn app_mutex(&self) -> Option<&str> {
        self.app_mutex.as_ref().map(PascalString::as_str)
    }

    /// Returns the default user info name for the application.
    #[doc(alias = "DefaultUserInfoName")]
    #[must_use]
    pub fn default_user_name(&self) -> Option<&str> {
        self.default_user_name.as_ref().map(PascalString::as_str)
    }

    /// Returns the default user organization for the application.
    #[doc(alias = "DefaultUserInfoOrg")]
    #[must_use]
    pub fn default_user_organization(&self) -> Option<&str> {
        self.default_user_organisation
            .as_ref()
            .map(PascalString::as_str)
    }

    /// Returns the default serial number for the application.
    #[doc(alias = "DefaultUserInfoSerial")]
    #[must_use]
    pub fn default_serial(&self) -> Option<&str> {
        self.default_serial.as_ref().map(PascalString::as_str)
    }

    /// Returns the name of the application `ReadMe` file.
    #[doc(alias = "AppReadmeFile")]
    #[must_use]
    pub fn app_readme_file(&self) -> Option<&str> {
        self.app_readme_file.as_ref().map(PascalString::as_str)
    }

    /// Returns the application contact.
    #[doc(alias = "AppContact")]
    #[must_use]
    pub fn app_contact(&self) -> Option<&str> {
        self.app_contact.as_ref().map(PascalString::as_str)
    }

    /// Returns the application comments.
    #[doc(alias = "AppComments")]
    #[must_use]
    pub fn app_comments(&self) -> Option<&str> {
        self.app_comments.as_ref().map(PascalString::as_str)
    }

    /// Returns the application modify path.
    #[doc(alias = "AppModifyPath")]
    #[must_use]
    pub fn app_modify_path(&self) -> Option<&str> {
        self.app_modify_path.as_ref().map(PascalString::as_str)
    }

    /// Returns `true` if the application creates an uninstallation registry key.
    #[doc(alias = "CreateUninstallRegistryKey")]
    #[must_use]
    pub fn create_uninstall_registry_key(&self) -> bool {
        self.create_uninstall_registry_key
            .as_ref()
            .is_some_and(|create| YesNoStr::new(create.as_str()).as_bool())
    }

    /// Returns `true` if the application is uninstallable.
    #[doc(alias = "Uninstallable")]
    #[must_use]
    pub fn is_uninstallable(&self) -> bool {
        self.uninstallable
            .as_ref()
            .is_some_and(|uninstallable| YesNoStr::new(uninstallable.as_str()).as_bool())
    }

    /// Returns the close applications filter.
    #[doc(alias = "CloseApplicationsFilter")]
    #[must_use]
    pub fn close_applications_filter(&self) -> bool {
        self.close_applications_filter
            .as_ref()
            .is_some_and(|close| YesNoStr::new(close.as_str()).as_bool())
    }

    /// Returns the name of the setup mutex.
    #[doc(alias = "SetupMutex")]
    #[must_use]
    pub fn setup_mutex(&self) -> Option<&str> {
        self.setup_mutex.as_ref().map(PascalString::as_str)
    }

    /// Returns `true` if the application installation changes the environment.
    #[must_use]
    #[doc(alias = "ChangesEnvironment")]
    pub fn changes_environment(&self) -> bool {
        self.changes_environment
            .as_ref()
            .is_some_and(|close| YesNoStr::new(close.as_str()).as_bool())
    }

    /// Returns `true` if the application installation changes file associations.
    #[must_use]
    #[doc(alias = "ChangesAssociations")]
    pub fn changes_associations(&self) -> bool {
        self.changes_associations
            .as_ref()
            .is_some_and(|close| YesNoStr::new(close.as_str()).as_bool())
    }

    /// Returns the architectures of the systems that the installer is allowed to install on.
    #[doc(alias = "ArchitecturesAllowed")]
    #[must_use]
    pub fn architectures_allowed(&self) -> Architecture {
        self.architectures_allowed_expr
            .as_ref()
            .map_or(self.architectures_allowed, |expr| {
                let (allowed, _disallowed) = Architecture::from_expression(expr.as_str());
                allowed
            })
    }

    /// Returns the architectures that the installer is not allowed to install on.
    #[must_use]
    pub fn architectures_disallowed(&self) -> Architecture {
        self.architectures_allowed_expr
            .as_ref()
            .map_or(self.architectures_disallowed, |expr| {
                let (_allowed, disallowed) = Architecture::from_expression(expr.as_str());
                disallowed
            })
    }

    /// Returns the architectures on which Setup should enable [64-bit install mode].
    ///
    /// By default, Setup will always use [32-bit install mode].
    ///
    /// [32-bit install mode]: https://jrsoftware.org/ishelp/topic_32vs64bitinstalls.htm
    /// [64-bit install mode]: https://jrsoftware.org/ishelp/topic_32vs64bitinstalls.htm
    #[doc(alias = "ArchitecturesInstallIn64BitMode")]
    #[must_use]
    pub fn architectures_install_in_64_bit_mode(&self) -> Architecture {
        self.architectures_install_in_64_bit_mode_expr
            .as_ref()
            .map_or(self.architectures_install_in_64_bit_mode, |expr| {
                let (allowed, _disallowed) = Architecture::from_expression(expr.as_str());
                allowed
            })
    }

    /// Returns the close applications filter excludes.
    #[doc(alias = "CloseApplicationsFilterExcludes")]
    #[must_use]
    pub fn close_applications_filter_excludes(&self) -> Option<&str> {
        self.close_applications_filter_excludes
            .as_ref()
            .map(PascalString::as_str)
    }

    /// Returns the 7-zip library name.
    #[doc(alias = "SevenZipLibraryName")]
    #[must_use]
    pub fn seven_zip_library_name(&self) -> Option<&str> {
        self.seven_zip_library_name
            .as_ref()
            .map(PascalString::as_str)
    }

    /// Returns whether to use the previous app directory.
    #[doc(alias = "UsePreviousAppDir")]
    #[must_use]
    pub fn use_previous_app_dir(&self) -> Option<&str> {
        self.use_previous_app_dir.as_ref().map(PascalString::as_str)
    }

    /// Returns whether to use the previous group.
    #[doc(alias = "UsePreviousGroup")]
    #[must_use]
    pub fn use_previous_group(&self) -> Option<&str> {
        self.use_previous_group.as_ref().map(PascalString::as_str)
    }

    /// Returns whether to use the previous setup type.
    #[doc(alias = "UsePreviousSetupType")]
    #[must_use]
    pub fn use_previous_setup_type(&self) -> Option<&str> {
        self.use_previous_setup_type
            .as_ref()
            .map(PascalString::as_str)
    }

    /// Returns whether to use the previous setup type.
    #[doc(alias = "UsePreviousTasks")]
    #[must_use]
    pub fn use_previous_tasks(&self) -> Option<&str> {
        self.use_previous_tasks.as_ref().map(PascalString::as_str)
    }

    /// Returns whether to use the previous user info
    #[doc(alias = "UsePreviousUserInfo")]
    #[must_use]
    pub fn use_previous_user_info(&self) -> Option<&str> {
        self.use_previous_user_info
            .as_ref()
            .map(PascalString::as_str)
    }

    /// Returns the license text.
    #[doc(alias = "LicenseText")]
    #[must_use]
    pub fn license_text(&self) -> Option<&str> {
        self.license_text.as_deref()
    }

    /// Returns the info before text.
    #[doc(alias = "InfoBeforeText")]
    #[must_use]
    pub fn info_before(&self) -> Option<&str> {
        self.info_before.as_deref()
    }

    /// Returns the info after text.
    #[doc(alias = "InfoAfterText")]
    #[must_use]
    pub fn info_after(&self) -> Option<&str> {
        self.info_after.as_deref()
    }

    /// Returns the uninstaller signature.
    #[doc(alias = "UninstallerSignature")]
    #[must_use]
    pub fn uninstaller_signature(&self) -> Option<&str> {
        self.uninstaller_signature.as_deref()
    }

    /// Returns the compiled code text.
    #[doc(alias = "CompiledCodeText")]
    #[must_use]
    pub fn compiled_code_text(&self) -> Option<&str> {
        self.compiled_code.as_deref()
    }

    /// Returns the number of language entries.
    #[doc(alias = "NumLanguageEntries")]
    #[must_use]
    #[inline]
    pub const fn language_count(&self) -> u32 {
        self.entry_counts.language()
    }

    /// Returns the number of message entries.
    #[doc(alias = "NumCustomMessageEntries")]
    #[must_use]
    #[inline]
    pub const fn custom_message_count(&self) -> u32 {
        self.entry_counts.custom_message()
    }

    /// Returns the number of permission entries.
    #[doc(alias = "NumPermissionEntries")]
    #[must_use]
    #[inline]
    pub const fn permission_count(&self) -> u32 {
        self.entry_counts.permission()
    }

    /// Returns the number of type entries.
    #[doc(alias = "NumTypeEntries")]
    #[must_use]
    #[inline]
    pub const fn type_count(&self) -> u32 {
        self.entry_counts.r#type()
    }

    /// Returns the number of component entries.
    #[doc(alias = "NumComponentEntries")]
    #[must_use]
    #[inline]
    pub const fn component_count(&self) -> u32 {
        self.entry_counts.component()
    }

    /// Returns the number of task entries.
    #[doc(alias = "NumTaskEntries")]
    #[must_use]
    #[inline]
    pub const fn task_count(&self) -> u32 {
        self.entry_counts.task()
    }

    /// Returns the number of directory entries.
    #[doc(alias = "NumDirEntries")]
    #[must_use]
    #[inline]
    pub const fn directory_count(&self) -> u32 {
        self.entry_counts.directory()
    }

    /// Returns the number of IS Sig Key Entries.
    #[doc(alias = "NumISSigKeyEntries")]
    #[must_use]
    #[inline]
    pub const fn is_sig_keys_count(&self) -> u32 {
        self.entry_counts.is_sig_key()
    }

    /// Returns the number of file entries.
    #[doc(alias = "NumFileEntries")]
    #[must_use]
    #[inline]
    pub const fn file_count(&self) -> u32 {
        self.entry_counts.file()
    }

    /// Returns the number of file location entries.
    #[doc(alias = "NumFileLocationEntries")]
    #[must_use]
    #[inline]
    pub const fn file_location_entry_count(&self) -> u32 {
        self.entry_counts.file_location()
    }

    /// Returns the number of icon entries.
    #[doc(alias = "NumIconEntries")]
    #[must_use]
    #[inline]
    pub const fn icon_count(&self) -> u32 {
        self.entry_counts.icon()
    }

    /// Returns the number of ini entries.
    #[doc(alias = "NumIniEntries")]
    #[must_use]
    #[inline]
    pub const fn ini_entry_count(&self) -> u32 {
        self.entry_counts.ini()
    }

    /// Returns the number of registry entries.
    #[doc(alias = "NumRegistryEntries")]
    #[must_use]
    #[inline]
    pub const fn registry_entry_count(&self) -> u32 {
        self.entry_counts.registry()
    }

    /// Returns the number of install delete entries.
    #[doc(alias = "NumInstallDeleteEntries")]
    #[must_use]
    #[inline]
    pub const fn install_delete_entry_count(&self) -> u32 {
        self.entry_counts.install_delete()
    }

    /// Returns the number of uninstall delete entries.
    #[doc(alias = "NumUninstallDeleteEntries")]
    #[must_use]
    #[inline]
    pub const fn uninstall_delete_entry_count(&self) -> u32 {
        self.entry_counts.uninstall_delete()
    }

    /// Returns the number of run entries.
    #[doc(alias = "NumRunEntries")]
    #[must_use]
    #[inline]
    pub const fn run_entry_count(&self) -> u32 {
        self.entry_counts.run()
    }

    /// Returns the number of uninstall run entries.
    #[doc(alias = "NumUninstallRunEntries")]
    #[must_use]
    #[inline]
    pub const fn uninstall_run_entry_count(&self) -> u32 {
        self.entry_counts.uninstall_run()
    }

    /// Returns the compiled code version.
    ///
    /// Added in Inno Setup 7.0.
    #[doc(alias = "CompiledCodeVersion")]
    #[must_use]
    #[inline]
    pub const fn compiled_code_version(&self) -> Option<u32> {
        self.compiled_code_version
    }

    /// Returns the background color.
    #[must_use]
    #[inline]
    pub const fn background_color(&self) -> Color {
        self.background_color
    }

    /// Returns the 2nd background color.
    #[must_use]
    #[inline]
    pub const fn background_color2(&self) -> Color {
        self.background_color2
    }

    /// Returns the image background color.
    #[must_use]
    #[inline]
    pub const fn image_background_color(&self) -> Color {
        self.wizard.image_back_color()
    }

    /// Returns the small image background color.
    #[must_use]
    #[inline]
    pub const fn small_image_background_color(&self) -> Color {
        self.wizard.small_image_back_color()
    }

    /// Returns the image background color used in dark mode when a dynamic theme is enabled.
    #[must_use]
    #[inline]
    pub const fn image_dynamic_background_color(&self) -> Color {
        self.wizard.image_back_color_dynamic_dark()
    }

    /// Returns the small image background color used in dark mode when a dynamic theme is enabled.
    #[must_use]
    #[inline]
    pub const fn small_image_dynamic_background_color(&self) -> Color {
        self.wizard.small_image_back_color_dynamic_dark()
    }

    /// Returns the wizard image opacity.
    #[must_use]
    #[inline]
    pub const fn wizard_image_opacity(&self) -> Option<u8> {
        self.wizard.image_opacity()
    }

    /// Returns the wizard style.
    #[doc(alias = "WizardStyle")]
    #[must_use]
    #[inline]
    pub const fn wizard_style(&self) -> WizardStyle {
        self.wizard.style()
    }

    /// Returns the wizard size percent (horizontal, vertical).
    #[doc(alias = "WizardSizePercent")]
    #[must_use]
    #[inline]
    pub const fn wizard_size_percent(&self) -> WizardSizePercent {
        self.wizard.size_percent()
    }

    /// Returns the image alpha format.
    #[doc(alias = "WizardImageAlphaFormat")]
    #[must_use]
    #[inline]
    pub const fn wizard_image_alpha_format(&self) -> ImageAlphaFormat {
        self.wizard.image_alpha_format()
    }

    /// Returns the encryption header.
    ///
    /// This was removed from the Setup Header in Inno Setup 6.5.0 and is instead placed before the
    /// Inno Stream.
    #[must_use]
    #[inline]
    pub const fn encryption_header(&self) -> Option<&EncryptionHeader> {
        self.encryption_header.as_ref()
    }

    /// Returns the extra disk space required for the installation.
    #[doc(alias = "ExtraDiskSpaceRequired")]
    #[must_use]
    #[inline]
    pub const fn extra_disk_space_required(&self) -> u64 {
        self.extra_disk_space_required
    }

    /// Returns the slices per disk.
    #[doc(alias = "SlicesPerDisk")]
    #[must_use]
    #[inline]
    pub const fn slices_per_disk(&self) -> u32 {
        self.slices_per_disk.get()
    }

    pub(crate) const fn slices_per_disk_checked(&self) -> SlicesPerDisk {
        self.slices_per_disk
    }

    /// Returns the install verbosity.
    #[doc(alias = "UninstallLogMode")]
    #[must_use]
    #[inline]
    pub const fn install_verbosity(&self) -> InstallVerbosity {
        self.install_verbosity
    }

    /// Returns the uninstallation log mode.
    #[doc(alias = "UninstallLogMode")]
    #[must_use]
    #[inline]
    pub const fn uninstall_log_mode(&self) -> LogMode {
        self.uninstall_log_mode
    }

    /// Returns the uninstallation style.
    ///
    /// In Inno Setup v5 and above, this is always `Modern`.
    #[must_use]
    #[inline]
    pub const fn uninstall_style(&self) -> WizardStyle {
        self.uninstall_style
    }

    /// Returns the mode of the directory exists warning
    #[doc(alias = "DirExistsWarning")]
    #[must_use]
    #[inline]
    pub const fn directory_exists_warning(&self) -> AutoBool {
        self.dir_exists_warning
    }

    /// Returns the privileges required for the installation.
    ///
    /// This directive affects whether elevated rights are requested (via a User Account Control
    /// dialog) when the installation is started.
    ///
    /// See [`PrivilegeLevel`].
    #[doc(alias = "PrivilegesRequired")]
    #[must_use]
    #[inline]
    pub const fn privileges_required(&self) -> PrivilegeLevel {
        self.privileges_required
    }

    /// Returns the enabled overrides, if any, which allow the end user to override the script's
    /// default [PrivilegesRequired] setting.
    ///
    /// [PrivilegesRequired]: https://jrsoftware.org/ishelp/topic_setup_privilegesrequired.htm
    #[doc(alias = "PrivilegesRequiredOverridesAllowed")]
    #[must_use]
    #[inline]
    pub const fn privileges_required_overrides_allowed(&self) -> PrivilegesRequiredOverrides {
        self.privileges_required_overrides_allowed
    }

    #[doc(alias = "ShowLanguageDialog")]
    #[must_use]
    #[inline]
    pub const fn show_language_dialog(&self) -> AutoBool {
        self.show_language_dialog
    }

    /// Returns the language detection method used in the installation.
    #[doc(alias = "LanguageDetectionMethod")]
    #[must_use]
    #[inline]
    pub const fn language_detection_method(&self) -> LanguageDetection {
        self.language_detection
    }

    /// Returns the compression method used by the installer.
    #[doc(alias = "CompressMethod")]
    #[must_use]
    #[inline]
    pub const fn compression(&self) -> Compression {
        self.compression
    }

    /// Returns the signed uninstaller original size.
    #[doc(alias = "SignedUninstallerOriginalSize")]
    #[must_use]
    #[inline]
    pub const fn signed_uninstaller_original_size(&self) -> u32 {
        self.signed_uninstaller_original_size
    }

    /// Returns the signed uninstaller header checksum.
    #[doc(alias = "SignedUninstallerHeaderChecksum")]
    #[must_use]
    #[inline]
    pub const fn signed_uninstaller_header_checksum(&self) -> u32 {
        self.signed_uninstaller_header_checksum
    }

    /// Returns whether the directory page is disabled.
    #[doc(alias = "DisableDirPage")]
    #[must_use]
    #[inline]
    pub const fn is_directory_page_disabled(&self) -> AutoBool {
        self.disable_dir_page
    }

    #[doc(alias = "DisableProgramGroupPage")]
    #[must_use]
    #[inline]
    pub const fn is_program_group_page_disabled(&self) -> AutoBool {
        self.disable_program_group_page
    }

    #[doc(alias = "UninstallDisplaySize")]
    #[must_use]
    #[inline]
    pub const fn uninstall_display_size(&self) -> u64 {
        self.uninstall_display_size
    }
}

fn password_salt<const LEN: usize>(reader: &mut impl io::Read) -> io::Result<String> {
    let mut password_salt_buf = [0; LEN];
    reader.read_exact(&mut password_salt_buf)?;
    Ok(String::from_utf8_lossy(&password_salt_buf).into_owned())
}

impl fmt::Debug for Header {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Header")
            .field("Flags", &self.flags)
            .field("AppName", &self.app_name())
            .field("AppVerName", &self.app_versioned_name())
            .field("AppId", &self.app_id())
            .field("AppCopyright", &self.app_copyright())
            .field("AppPublisher", &self.app_publisher())
            .field("AppPublisherURL", &self.app_publisher_url())
            .field("AppSupportPhone", &self.app_support_phone())
            .field("AppSupportURL", &self.app_support_url())
            .field("AppUpdatesURL", &self.app_updates_url())
            .field("AppVersion", &self.app_version())
            .field("DefaultDirName", &self.default_dir_name())
            .field("DefaultGroupName", &self.default_group_name())
            .field("UninstallIconName", &self.uninstall_icon_name())
            .field("BaseFileName", &self.base_filename())
            .field("UninstallFilesDir", &self.uninstall_files_dir())
            .field("UninstallName", &self.uninstall_name())
            .field("UninstallIcon", &self.uninstall_icon())
            .field("AppMutex", &self.app_mutex())
            .field("DefaultUserInfoName", &self.default_user_name())
            .field("DefaultUserInfoOrg", &self.default_user_organization())
            .field("DefaultUserInfoSerial", &self.default_serial())
            .field("AppReadmeFile", &self.app_readme_file())
            .field("AppContact", &self.app_contact())
            .field("AppComments", &self.app_comments())
            .field("AppModifyPath", &self.app_modify_path())
            .field(
                "CreateUninstallRegKey",
                &self.create_uninstall_registry_key(),
            )
            .field("Uninstallable", &self.is_uninstallable())
            .field("CloseApplicationsFilter", &self.close_applications_filter())
            .field("SetupMutex", &self.setup_mutex())
            .field("ChangesEnvironment", &self.changes_environment())
            .field("ChangesAssociations", &self.changes_associations())
            .field("ArchitecturesAllowed", &self.architectures_allowed())
            .field("ArchitecturesDisallowed", &self.architectures_disallowed())
            .field(
                "ArchitecturesInstallIn64BitMode",
                &self.architectures_install_in_64_bit_mode(),
            )
            .field(
                "CloseApplicationsFilterExcludes",
                &self.close_applications_filter_excludes(),
            )
            .field("SevenZipLibraryName", &self.seven_zip_library_name())
            .field("UsePreviousAppDir", &self.use_previous_app_dir())
            .field("UsePreviousGroup", &self.use_previous_group())
            .field("UsePreviousSetupType", &self.use_previous_setup_type())
            .field("UsePreviousTasks", &self.use_previous_tasks())
            .field("UsePreviousUserInfo", &self.use_previous_user_info())
            .field("LicenseText", &self.license_text())
            .field("InfoBeforeText", &self.info_before())
            .field("InfoAfterText", &self.info_after())
            .field("UninstallerSignature", &self.uninstaller_signature())
            // Skip compiled code text
            // Skip lead bytes
            .field("NumLanguageEntries", &self.language_count())
            .field("NumCustomMessageEntries", &self.custom_message_count())
            .field("NumPermissionEntries", &self.permission_count())
            .field("NumTypeEntries", &self.type_count())
            .field("NumComponentEntries", &self.component_count())
            .field("NumTaskEntries", &self.task_count())
            .field("NumDirEntries", &self.directory_count())
            .field("NumFileEntries", &self.file_count())
            .field("NumFileLocationEntries", &self.file_location_entry_count())
            .field("NumIconEntries", &self.icon_count())
            .field("NumIniEntries", &self.ini_entry_count())
            .field("NumRegistryEntries", &self.registry_entry_count())
            .field(
                "NumUninstallDeleteEntries",
                &self.uninstall_delete_entry_count(),
            )
            .field(
                "NumUninstallRunEntries",
                &self.uninstall_delete_entry_count(),
            )
            .field("CompiledCodeVersion", &self.compiled_code_version())
            .field("RunEntryCount", &self.run_entry_count())
            .field("UninstallRunEntryCount", &self.uninstall_run_entry_count())
            .field("BackColor", &self.background_color())
            .field("BackColor2", &self.background_color2())
            .field("WizardImageBackColor", &self.image_background_color())
            .field(
                "WizardSmallImageBackColor",
                &self.small_image_background_color(),
            )
            .field(
                "WizardImageBackColorDynamicDark",
                &self.image_dynamic_background_color(),
            )
            .field(
                "WizardSmallImageBackColorDynamicDark",
                &self.small_image_dynamic_background_color(),
            )
            .field("WizardStyle", &self.wizard_style())
            .field("WizardSizePercent", &self.wizard_size_percent())
            .field("ImageAlphaFormat", &self.wizard_image_alpha_format())
            // Skip password salt
            .field("ExtraDiskSpaceRequired", &self.extra_disk_space_required())
            .field("SlicesPerDisk", &self.slices_per_disk())
            .field("InstallVerbosity", &self.install_verbosity())
            .field("UninstallLogMode", &self.uninstall_log_mode())
            .field("UninstallStyle", &self.uninstall_style())
            .field("DirExistsWarning", &self.directory_exists_warning())
            .field("PrivilegesRequired", &self.privileges_required())
            .field(
                "PrivilegesRequiredOverridesAllowed",
                &self.privileges_required_overrides_allowed(),
            )
            .field("ShowLanguageDialog", &self.show_language_dialog())
            .field("LanguageDetection", &self.language_detection_method())
            .field("Compression", &self.compression())
            .field(
                "SignedUninstallerOriginalSize",
                &self.signed_uninstaller_original_size(),
            )
            .field(
                "SignedUninstallerHeaderChecksum",
                &self.signed_uninstaller_header_checksum(),
            )
            .field("DisableDirPage", &self.is_directory_page_disabled())
            .field(
                "DisableProgramGroupPage",
                &self.is_program_group_page_disabled(),
            )
            .field("UninstallDisplaySize", &self.uninstall_display_size())
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use super::Header;
    use crate::string::PascalString;

    #[test]
    fn product_code() {
        let header = Header {
            app_id: Some(PascalString::from(
                "{{31AA9DE2-36A2-4FB7-921F-865D4B0657D5}",
            )),
            ..Header::default()
        };

        assert_eq!(
            header.product_code().as_deref(),
            Some("{31AA9DE2-36A2-4FB7-921F-865D4B0657D5}_is1")
        );
    }
}
