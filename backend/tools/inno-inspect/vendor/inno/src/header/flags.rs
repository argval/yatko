use std::fmt;

use bitflags::bitflags;

bitflags! {
    #[derive(Clone, Copy, Default, Eq, PartialEq)]
    pub struct HeaderFlags: u128 {
        const DISABLE_STARTUP_PROMPT = 1;
        const CREATE_APP_DIR = 1 << 1;
        const ALLOW_NO_ICONS = 1 << 2;
        const ALWAYS_RESTART = 1 << 3;
        const ALWAYS_USE_PERSONAL_GROUP = 1 << 4;
        const WINDOW_VISIBLE = 1 << 5;
        const WINDOW_SHOW_CAPTION = 1 << 6;
        const WINDOW_RESIZABLE = 1 << 7;
        const WINDOW_START_MAXIMISED = 1 << 8;
        const ENABLED_DIR_DOESNT_EXIST_WARNING = 1 << 9;
        const PASSWORD = 1 << 10;
        const ALLOW_ROOT_DIRECTORY = 1 << 11;
        const DISABLE_FINISHED_PAGE = 1 << 12;
        const CHANGES_ASSOCIATIONS = 1 << 13;
        const BACK_COLOR_HORIZONTAL = 1 << 14;
        const UPDATE_UNINSTALL_LOG_APP_NAME = 1 << 15;
        const DISABLE_READY_MEMO = 1 << 16;
        const ALWAYS_SHOW_COMPONENTS_LIST = 1 << 17;
        const FLAT_COMPONENTS_LIST = 1 << 18;
        const SHOW_COMPONENT_SIZES = 1 << 19;
        const DISABLE_READY_PAGE = 1 << 20;
        const ALWAYS_SHOW_DIR_ON_READY_PAGE = 1 << 21;
        const ALWAYS_SHOW_GROUP_ON_READY_PAGE = 1 << 22;
        const ALLOW_UNC_PATH = 1 << 23;
        const USER_INFO_PAGE = 1 << 24;
        const UNINSTALL_RESTART_COMPUTER = 1 << 25;
        const RESTART_IF_NEEDED_BY_RUN = 1 << 26;
        const SHOW_TASKS_TREE_LINES = 1 << 27;
        const ALLOW_CANCEL_DURING_INSTALL = 1 << 28;
        const WIZARD_IMAGE_STRETCH = 1 << 29;
        const APPEND_DEFAULT_DIR_NAME = 1 << 30;
        const APPEND_DEFAULT_GROUP_NAME = 1 << 31;
        const ENCRYPTION_USED = 1 << 32;
        const CHANGES_ENVIRONMENT = 1 << 33;
        const SETUP_LOGGING = 1 << 34;
        const SIGNED_UNINSTALLER = 1 << 45;
        const USE_PREVIOUS_LANGUAGE = 1 << 46;
        const DISABLE_WELCOME_PAGE = 1 << 47;
        const CLOSE_APPLICATIONS = 1 << 48;
        const RESTART_APPLICATIONS = 1 << 49;
        const ALLOW_NETWORK_DRIVE = 1 << 50;
        const FORCE_CLOSE_APPLICATIONS = 1 << 51;
        const APP_NAME_HAS_CONSTS = 1 << 52;
        const USE_PREVIOUS_PRIVILEGES = 1 << 53;
        const WIZARD_RESIZABLE = 1 << 54;
        const UNINSTALL_LOGGING = 1 << 55;
        const WIZARD_MODERN = 1 << 56;
        const WIZARD_BORDER_STYLED = 1 << 57;
        const WIZARD_KEEP_ASPECT_RATIO = 1 << 58;
        const REDIRECTION_GUARD = 1 << 59;
        const WIZARD_BEVELS_HIDDEN = 1 << 60;
        const PADDING = 1 << 61;

        // ~~~Obsolete flags~~~

        const UNINSTALLABLE = 1 << 108;
        const DISABLE_DIR_PAGE = 1 << 109;
        const DISABLE_PROGRAM_GROUP_PAGE = 1 << 110;
        const DISABLE_APPEND_DIR = 1 << 111;
        const ADMIN_PRIVILEGES_REQUIRED = 1 << 112;
        const ALWAYS_CREATE_UNINSTALL_ICON = 1 << 113;
        const CREATE_UNINSTALL_REG_KEY = 1 << 114;
        const BZIP_USED = 1 << 115;
        const SHOW_LANGUAGE_DIALOG = 1 << 116;
        const DETECT_LANGUAGE_USING_LOCALE = 1 << 117;
        const DISABLE_DIR_EXISTS_WARNING = 1 << 118;
        const BACK_SOLID = 1 << 119;
        const OVERWRITE_UNINSTALL_REG_ENTRIES = 1 << 120;
        const SHOW_UNDISPLAYABLE_LANGUAGES = 1 << 121;

        // ~~~Removed in 6.7.0~~~
        const USE_PREVIOUS_APP_DIR = 1 << 122;
        const USE_PREVIOUS_GROUP = 1 << 123;
        const USE_PREVIOUS_SETUP_TYPE = 1 << 124;
        const USE_PREVIOUS_TASKS = 1 << 125;
        const USE_PREVIOUS_USER_INFO = 1 << 126;
        const WIZARD_LIGHT_BUTTONS_UNSTYLED = 1 << 127;
    }
}

impl fmt::Debug for HeaderFlags {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.is_empty() {
            f.write_str("0x0")
        } else {
            bitflags::parser::to_writer(self, f)
        }
    }
}
