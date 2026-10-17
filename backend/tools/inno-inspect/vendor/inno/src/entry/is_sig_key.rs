use std::io;

use encoding_rs::Encoding;

use crate::read::ReadBytesExt;

/// <https://github.com/jrsoftware/issrc/blob/is-6_5_1/Projects/Src/Shared.Struct.pas#L232>
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct ISSigKey {
    public_x: Option<String>,
    public_y: Option<String>,
    runtime_id: Option<String>,
}

impl ISSigKey {
    pub fn read<R>(mut reader: R, codepage: &'static Encoding) -> io::Result<Self>
    where
        R: io::Read,
    {
        Ok(Self {
            public_x: reader.read_decoded_pascal_string(codepage)?,
            public_y: reader.read_decoded_pascal_string(codepage)?,
            runtime_id: reader.read_decoded_pascal_string(codepage)?,
        })
    }

    /// Returns the Public X value.
    #[must_use]
    #[inline]
    pub fn public_x(&self) -> Option<&str> {
        self.public_x.as_deref()
    }

    /// Returns the Public Y value.
    #[must_use]
    #[inline]
    pub fn public_y(&self) -> Option<&str> {
        self.public_y.as_deref()
    }

    /// Returns the Runtime ID.
    #[must_use]
    #[inline]
    pub fn runtime_id(&self) -> Option<&str> {
        self.runtime_id.as_deref()
    }
}
