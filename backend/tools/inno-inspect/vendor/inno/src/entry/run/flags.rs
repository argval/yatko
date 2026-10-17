use std::fmt;

use bitflags::bitflags;

bitflags! {
    #[derive(Clone, Copy, Default, Eq, PartialEq)]
    pub struct RunFlags: u16 {
        const SHELL_EXECUTE = 1;
        const SKIP_IF_DOESNT_EXIST = 1 << 1;
        const POST_INSTALL = 1 << 2;
        const UNCHECKED = 1 << 3;
        const SKIP_IF_SILENT = 1 << 4;
        const SKIP_IF_NOT_SILENT = 1 << 5;
        const HIDE_WIZARD = 1 << 6;
        const BITS_32 = 1 << 7;
        const BITS_64 = 1 << 8;
        const RUN_AS_ORIGINAL_USER = 1 << 9;
        const DONT_LOG_PARAMETERS = 1 << 10;
        const LOG_OUTPUT = 1 << 11;
    }
}

impl fmt::Debug for RunFlags {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.is_empty() {
            f.write_str("None")
        } else {
            bitflags::parser::to_writer(self, f)
        }
    }
}
