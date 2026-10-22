mod flags;
mod wait_condition;

use std::io;

use encoding_rs::Encoding;
pub use flags::RunFlags;
pub use wait_condition::WaitCondition;
use zerocopy::LE;

use crate::{
    entry::{Bitness, Condition},
    header::flag_reader::read_flags::read_flags,
    read::ReadBytesExt,
    version::{InnoVersion, windows_version::WindowsVersionRange},
};

#[derive(Clone, Debug, Default)]
pub struct RunEntry {
    name: Option<String>,
    parameters: Option<String>,
    working_directory: Option<String>,
    run_once_id: Option<String>,
    status_message: Option<String>,
    verb: Option<String>,
    description: Option<String>,
    /// Added in Inno Setup 7.0.0.1.
    on_log: Option<String>,
    show_command: i32,
    wait_condition: WaitCondition,
    bitness: Bitness,
    options: RunFlags,
}

impl RunEntry {
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

        let mut run_entry = Self {
            name: reader.read_decoded_pascal_string(codepage)?,
            parameters: reader.read_decoded_pascal_string(codepage)?,
            working_directory: reader.read_decoded_pascal_string(codepage)?,
            ..Self::default()
        };

        if version >= (1, 3, 9) {
            run_entry.run_once_id = reader.read_decoded_pascal_string(codepage)?;
        }

        if version >= (2, 0, 2) {
            run_entry.status_message = reader.read_decoded_pascal_string(codepage)?;
        }

        if version >= (5, 1, 13) {
            run_entry.verb = reader.read_decoded_pascal_string(codepage)?;
        }

        if version >= 2 || version.is_isx() {
            run_entry.description = reader.read_decoded_pascal_string(codepage)?;
        }

        Condition::read(&mut reader, codepage, version)?;

        if version >= (7, 0, 0, 1) {
            run_entry.on_log = reader.read_decoded_pascal_string(codepage)?;
        }

        WindowsVersionRange::read_from(&mut reader, version)?;

        if version >= (1, 3, 24) {
            run_entry.show_command = reader.read_i32::<LE>()?;
        }

        run_entry.wait_condition = WaitCondition::try_read_from_io(&mut reader)?;

        if version >= (7, 0, 0, 3) {
            run_entry.bitness = Bitness::try_read_from_io(&mut reader)?;
        }

        run_entry.options = read_flags!(&mut reader,
            if version >= (1, 2, 3) => RunFlags::SHELL_EXECUTE,
            if version >= (1, 3, 9) || (version.is_isx() && version >= (1, 3, 8)) => RunFlags::SKIP_IF_DOESNT_EXIST,
            if version >= 2 => [
                RunFlags::POST_INSTALL,
                RunFlags::UNCHECKED,
                RunFlags::SKIP_IF_SILENT,
                RunFlags::SKIP_IF_NOT_SILENT
            ],
            if version >= (2, 0, 8) => RunFlags::HIDE_WIZARD,
            if version >= (5, 1, 10) && version < (7, 0, 0, 3) => [RunFlags::BITS_32, RunFlags::BITS_64],
            if version >= 5.2 => RunFlags::RUN_AS_ORIGINAL_USER,
            if version >= 6.1 => RunFlags::DONT_LOG_PARAMETERS,
            if version >= 6.3 => RunFlags::LOG_OUTPUT,
        )?;

        if run_entry.bitness == Bitness::Bit32 {
            run_entry.options |= RunFlags::BITS_32;
        } else if run_entry.bitness == Bitness::Bit64 {
            run_entry.options |= RunFlags::BITS_64;
        }

        Ok(run_entry)
    }

    /// Returns the name of the run entry as a string slice.
    #[must_use]
    #[inline]
    pub fn name(&self) -> Option<&str> {
        self.name.as_deref()
    }

    /// Returns parameters of the run entry as a string slice.
    #[must_use]
    #[inline]
    pub fn parameters(&self) -> Option<&str> {
        self.parameters.as_deref()
    }

    /// Returns the working directory of the run entry as a string slice.
    #[must_use]
    #[inline]
    pub fn working_directory(&self) -> Option<&str> {
        self.working_directory.as_deref()
    }

    /// Returns the `RunOnceId` of the run entry as a string slice.
    #[must_use]
    #[inline]
    pub fn run_once_id(&self) -> Option<&str> {
        self.run_once_id.as_deref()
    }

    /// Returns the status message of the run entry as a string slice.
    #[must_use]
    #[inline]
    pub fn status_message(&self) -> Option<&str> {
        self.status_message.as_deref()
    }

    /// Returns the [verb](https://learn.microsoft.com/windows/win32/api/shellapi/ns-shellapi-shellexecuteinfow)
    /// of the run entry as a string slice.
    #[must_use]
    #[inline]
    pub fn verb(&self) -> Option<&str> {
        self.verb.as_deref()
    }

    /// Returns the description of the run entry as a string slice.
    #[must_use]
    #[inline]
    pub fn description(&self) -> Option<&str> {
        self.description.as_deref()
    }

    /// Returns the show command of the run entry.
    #[must_use]
    #[inline]
    pub const fn show_command(&self) -> i32 {
        self.show_command
    }

    /// Returns the wait condition of the run entry.
    #[must_use]
    #[inline]
    pub const fn wait_condition(&self) -> WaitCondition {
        self.wait_condition
    }

    /// Returns the bitness of the run entry.
    #[must_use]
    #[inline]
    pub const fn bitness(&self) -> Bitness {
        self.bitness
    }

    /// Returns the options of the run entry.
    #[must_use]
    #[inline]
    pub const fn options(&self) -> RunFlags {
        self.options
    }
}
