use std::fmt;

use bitflags::bitflags;

bitflags! {
    /// Can be set to one or more overrides which allow the end user to override the script's
    /// default [PrivilegesRequired] setting.
    ///
    /// See [PrivilegesRequiredOverridesAllowed].
    ///
    /// [PrivilegesRequired]: https://jrsoftware.org/ishelp/topic_setup_privilegesrequired.htm
    /// [PrivilegesRequiredOverridesAllowed]: https://jrsoftware.org/ishelp/index.php?topic=setup_privilegesrequiredoverridesallowed
    #[derive(Clone, Copy, Default, Eq, PartialEq)]
    pub struct PrivilegesRequiredOverrides: u8 {
        /// Setup will support two additional command line parameters to override the script's
        /// default [PrivilegesRequired] setting: `/ALLUSERS` and `/CURRENTUSER`. See [Setup Command
        /// Line Parameters] for more details.
        ///
        /// [PrivilegesRequired]: https://jrsoftware.org/ishelp/topic_setup_privilegesrequired.htm
        /// [Setup Command Line Parameters]: https://jrsoftware.org/ishelp/topic_setupcmdline.htm#ALLUSERS
        const COMMAND_LINE = 1;

        /// Setup will ask the user to choose the install mode based on the script's default
        /// [PrivilegesRequired] setting using a suppressible dialog. Allowing `dialog`
        /// automatically allows `commandline` and when one of the command line parameters is used
        /// then Setup will not ask the user.
        ///
        /// [PrivilegesRequired]: https://jrsoftware.org/ishelp/topic_setup_privilegesrequired.htm
        const DIALOG = 1 << 1;
    }
}

impl fmt::Debug for PrivilegesRequiredOverrides {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.is_empty() {
            f.write_str("0x0")
        } else {
            bitflags::parser::to_writer(self, f)
        }
    }
}
