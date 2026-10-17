use std::io;

use zerocopy::{Immutable, KnownLayout, LittleEndian, TryFromBytes, U32};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(u32)]
enum Magic {
    #[default]
    Inno = u32::from_le_bytes(*b"Inno"),
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Immutable, KnownLayout, TryFromBytes)]
#[repr(C)]
pub struct SetupLoaderOffset {
    magic: Magic,

    /// The offset to the setup loader table.
    table_offset: U32<LittleEndian>,

    /// The logical NOT of the table offset for validating the table offset.
    not_table_offset: U32<LittleEndian>,
}

impl SetupLoaderOffset {
    pub fn try_read<R>(mut reader: R) -> io::Result<Self>
    where
        Self: Sized,
        R: io::Read,
    {
        let mut buf = [0; size_of::<Self>()];

        reader.read_exact(&mut buf)?;

        Self::try_read_from_bytes(&buf)
            .map_err(|err| io::Error::new(io::ErrorKind::InvalidData, err.to_string()))
            .and_then(|setup_loader_offset| {
                if setup_loader_offset.is_valid() {
                    Ok(setup_loader_offset)
                } else {
                    Err(io::Error::new(
                        io::ErrorKind::InvalidData,
                        "Setup loader table offset does not equal the NOT table offset",
                    ))
                }
            })
    }

    /// Returns the table offset of the setup loader.
    #[must_use]
    #[inline]
    pub const fn table_offset(&self) -> u32 {
        self.table_offset.get()
    }

    /// Returns `true` if the table offset is valid, meaning it equals the logical NOT of the
    /// `not_table_offset`.
    #[must_use]
    #[inline]
    pub fn is_valid(&self) -> bool {
        self.table_offset == !self.not_table_offset
    }
}
