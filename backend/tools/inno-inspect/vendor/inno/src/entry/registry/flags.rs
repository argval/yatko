use std::fmt;

use bitflags::bitflags;

bitflags! {
    #[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
    pub struct RegistryFlags: u16 {
        const CREATE_VALUE_IF_DOESNT_EXIST = 1;
        const UNINSTALL_DELETE_VALUE = 1 << 1;
        const UNINSTALL_CLEAR_VALUE = 1 << 2;
        const UNINSTALL_DELETE_ENTIRE_KEY = 1 << 3;
        const UNINSTALL_DELETE_ENTIRE_KEY_IF_EMPTY = 1 << 4;
        const PRESERVE_STRING_TYPE = 1 << 5;
        const DELETE_KEY = 1 << 6;
        const DELETE_VALUE = 1 << 7;
        const NO_ERROR = 1 << 8;
        const DONT_CREATE_KEY = 1 << 9;
        const BITS_32 = 1 << 10;
        const BITS_64 = 1 << 11;
    }
}

impl fmt::Display for RegistryFlags {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        bitflags::parser::to_writer(self, f)
    }
}
