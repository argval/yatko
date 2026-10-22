mod target_type;

use std::io;

use encoding_rs::Encoding;
pub use target_type::TargetType;
use zerocopy::LE;

use super::Condition;
use crate::{InnoVersion, WindowsVersionRange, read::ReadBytesExt};

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct DeleteEntry {
    name: String,
    target_type: TargetType,
}

impl DeleteEntry {
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

        let mut delete_entry = Self {
            name: reader
                .read_decoded_pascal_string(codepage)?
                .unwrap_or_default(),
            ..Self::default()
        };

        Condition::read(&mut reader, codepage, version)?;

        WindowsVersionRange::read_from(&mut reader, version)?;

        delete_entry.target_type = TargetType::try_read_from_io(&mut reader)?;

        Ok(delete_entry)
    }

    /// Returns the name of the delete entry.
    #[must_use]
    #[inline]
    pub const fn name(&self) -> &str {
        self.name.as_str()
    }

    /// Returns the target type of the delete entry.
    #[must_use]
    #[inline]
    pub const fn target_type(&self) -> TargetType {
        self.target_type
    }
}
