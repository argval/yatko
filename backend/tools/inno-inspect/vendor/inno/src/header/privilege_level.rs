use std::{fmt, io};

use zerocopy::{Immutable, KnownLayout, TryFromBytes, ValidityError, try_transmute};

use super::HeaderFlags;

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u8)]
pub enum PrivilegeLevel {
    #[default]
    None = 0,
    PowerUser = 1,

    /// Setup will always run with administrative privileges and in [administrative install mode].
    /// If Setup was started by an unprivileged user, Windows will ask for the password to an
    /// account that has administrative privileges, and Setup will then run under that account.
    ///
    /// [administrative install mode]: https://jrsoftware.org/ishelp/topic_admininstallmode.htm
    Admin = 2,

    /// When set to `lowest`, Setup will not request to be run with administrative privileges even
    /// if it was started by a member of the Administrators group and will always run in
    /// [non-administrative install mode]. Do not use this setting unless you are sure your
    /// installation will run successfully on unprivileged accounts.
    ///
    /// [non-administrative install mode]: https://jrsoftware.org/ishelp/topic_admininstallmode.htm
    Lowest = 3,
}

impl PrivilegeLevel {
    pub fn try_read_from_io<R>(mut src: R) -> io::Result<Self>
    where
        Self: Sized,
        R: io::Read,
    {
        let mut buf = [0; size_of::<Self>()];
        src.read_exact(&mut buf)?;
        Self::try_read_from_bytes(&buf)
            .map_err(|err| io::Error::new(io::ErrorKind::InvalidData, err.to_string()))
    }

    #[must_use]
    pub const fn as_str(&self) -> &'static str {
        match self {
            Self::None => "None",
            Self::PowerUser => "Power User",
            Self::Admin => "Admin",
            Self::Lowest => "Lowest",
        }
    }
}

impl fmt::Display for PrivilegeLevel {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

impl From<HeaderFlags> for PrivilegeLevel {
    fn from(flags: HeaderFlags) -> Self {
        if flags.contains(HeaderFlags::ADMIN_PRIVILEGES_REQUIRED) {
            Self::Admin
        } else {
            Self::None
        }
    }
}

impl TryFrom<u8> for PrivilegeLevel {
    type Error = ValidityError<u8, Self>;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        try_transmute!(value)
    }
}
