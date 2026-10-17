mod r#type;

use std::io;

use encoding_rs::Encoding;
pub use r#type::FileVerificationType;

use crate::{entry::checksum::Sha256, read::ReadBytesExt};

/// <https://github.com/jrsoftware/issrc/blob/is-6_5_1/Projects/Src/Shared.Struct.pas#L241>
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct FileVerification {
    is_sig_allowed_keys: Option<String>,
    sha_256: Sha256,
    r#type: FileVerificationType,
}

impl FileVerification {
    pub fn read<R>(mut reader: R, codepage: &'static Encoding) -> io::Result<Self>
    where
        R: io::Read,
    {
        Ok(Self {
            is_sig_allowed_keys: reader.read_decoded_pascal_string(codepage)?,
            sha_256: reader.read_t::<Sha256>()?,
            r#type: FileVerificationType::try_read_from_io(reader)?,
        })
    }
}
