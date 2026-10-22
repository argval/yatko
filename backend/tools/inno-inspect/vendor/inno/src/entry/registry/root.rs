use std::fmt;

use zerocopy::{Immutable, KnownLayout, TryFromBytes};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u32)]
pub enum RegRoot {
    #[default]
    HKeyClassesRoot = 0,
    HKeyCurrentUser = 1u32.to_le(),
    HKeyLocalMachine = 2u32.to_le(),
    HKeyUsers = 3u32.to_le(),
    HKeyPerformanceData = 4u32.to_le(),
    HKeyCurrentConfig = 5u32.to_le(),
    HKeyDynamicData = 6u32.to_le(),
    Unset = 7u32.to_le(),
}

impl RegRoot {
    /// Returns the registry root as a static string.
    #[must_use]
    pub const fn as_str(&self) -> &'static str {
        match self {
            Self::HKeyClassesRoot => "HKEY_CLASSES_ROOT",
            Self::HKeyCurrentUser => "HKEY_CURRENT_USER",
            Self::HKeyLocalMachine => "HKEY_LOCAL_MACHINE",
            Self::HKeyUsers => "HKEY_USERS",
            Self::HKeyPerformanceData => "HKEY_PERFORMANCE_DATA",
            Self::HKeyCurrentConfig => "HKEY_CURRENT_CONFIG",
            Self::HKeyDynamicData => "HKEY_DYNAMIC_DATA",
            Self::Unset => "Unset",
        }
    }
}

impl fmt::Display for RegRoot {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}
