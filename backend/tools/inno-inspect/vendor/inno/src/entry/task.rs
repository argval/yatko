use std::{fmt, io};

use bitflags::bitflags;
use encoding_rs::Encoding;
use zerocopy::LE;

use crate::{
    InnoVersion, ReadBytesExt, WindowsVersionRange, header::flag_reader::read_flags::read_flags,
};

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct Task {
    name: Option<String>,
    description: Option<String>,
    group_description: Option<String>,
    components: Option<String>,
    languages: Option<String>,
    check: Option<String>,
    level: u32,
    used: bool,
    flags: TaskFlags,
}

impl Task {
    pub fn read<R>(
        mut reader: R,
        codepage: &'static Encoding,
        version: InnoVersion,
    ) -> io::Result<Self>
    where
        R: io::Read,
    {
        let mut task = Self {
            name: reader.read_decoded_pascal_string(codepage)?,
            description: reader.read_decoded_pascal_string(codepage)?,
            group_description: reader.read_decoded_pascal_string(codepage)?,
            components: reader.read_decoded_pascal_string(codepage)?,
            ..Self::default()
        };

        if version >= (4, 0, 1) {
            task.languages = reader.read_decoded_pascal_string(codepage)?;
        }

        if version >= 4 || (version.is_isx() && version >= (1, 3, 24)) {
            task.check = reader.read_decoded_pascal_string(codepage)?;
        }

        if version >= 6.7 {
            task.level = reader.read_u8()?.into();
        } else if version >= 4 || (version.is_isx() && version >= (3, 0, 3)) {
            task.level = reader.read_u32::<LE>()?;
        }

        if version >= 4 || (version.is_isx() && version >= (3, 0, 4)) {
            task.used = reader.read_u8()? != 0;
        } else {
            task.used = true;
        }

        WindowsVersionRange::read_from(&mut reader, version)?;

        task.flags = read_flags!(&mut reader,
            [TaskFlags::EXCLUSIVE, TaskFlags::UNCHECKED],
            if version >= (2, 0, 5) => TaskFlags::RESTART,
            if version >= (2, 0, 6) => TaskFlags::CHECKED_ONCE,
            if version >= (4, 2, 3) => TaskFlags::DONT_INHERIT_CHECK
        )?;

        Ok(task)
    }

    /// Returns the name of the task as a string slice.
    #[must_use]
    #[inline]
    pub fn name(&self) -> Option<&str> {
        self.name.as_deref()
    }

    /// Returns the description of the task as a string slice.
    #[must_use]
    #[inline]
    pub fn description(&self) -> Option<&str> {
        self.description.as_deref()
    }

    /// Returns the group description of the task as a string slice.
    #[must_use]
    #[inline]
    pub fn group_description(&self) -> Option<&str> {
        self.group_description.as_deref()
    }

    /// Returns the components of the task as a string slice.
    #[must_use]
    #[inline]
    pub fn components(&self) -> Option<&str> {
        self.components.as_deref()
    }

    /// Returns the languages of the task as a string slice.
    #[must_use]
    #[inline]
    pub fn languages(&self) -> Option<&str> {
        self.languages.as_deref()
    }

    /// Returns the check string of the task as a string slice.
    #[must_use]
    #[inline]
    pub fn check(&self) -> Option<&str> {
        self.check.as_deref()
    }

    /// Returns the level of the task.
    #[must_use]
    #[inline]
    pub const fn level(&self) -> u32 {
        self.level
    }

    /// Returns whether the task is used.
    #[must_use]
    #[inline]
    pub const fn used(&self) -> bool {
        self.used
    }

    /// Returns the flags of the task.
    #[must_use]
    #[inline]
    pub const fn flags(&self) -> TaskFlags {
        self.flags
    }
}

bitflags! {
    #[derive(Copy, Clone, Eq, PartialEq, Debug, Default)]
    pub struct TaskFlags: u8 {
        const EXCLUSIVE = 1;
        const UNCHECKED = 1 << 1;
        const RESTART = 1 << 2;
        const CHECKED_ONCE = 1 << 3;
        const DONT_INHERIT_CHECK = 1 << 4;
    }
}

impl fmt::Display for TaskFlags {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        bitflags::parser::to_writer(self, f)
    }
}
