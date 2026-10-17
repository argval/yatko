mod checksum;
mod offset;
mod signature;

use std::{
    io,
    io::{Read, Seek, SeekFrom},
};

pub use checksum::Checksum;
use offset::SetupLoaderOffset;
use signature::SetupLoaderSignature;
use zerocopy::LE;

use super::{
    InnoError, ReadBytesExt,
    entry::checksum::ChecksumMismatchError,
    error::InnoResult,
    pe::{
        CoffHeader, DosHeader, OptionalHeader, SectionTable, Signature,
        resource::{ImageResourceDataEntry, ResourceDirectory, SectionReader},
    },
    read::crc32::Crc32Reader,
    version::InnoVersion,
};

#[derive(Copy, Clone, Debug)]
pub struct SetupLoader {
    /// Signature of the setup loader.
    #[doc(alias = "ID")]
    signature: SetupLoaderSignature,

    /// Version of the setup loader.
    version: InnoVersion,

    /// Revision number of the setup loader.
    #[doc(alias = "SetupLdrOffsetTableVersion")]
    revision: u32,

    /// Minimum expected size of setup.exe.
    ///
    /// #### Revisions
    ///
    /// * 1: u32
    /// * 2: i64
    #[doc(alias = "TotalSize")]
    minimum_setup_exe_size: i64,

    /// Offset of compressed setup.e32.
    ///
    /// #### Revisions
    ///
    /// * 1: u32
    /// * 2: i64
    #[doc(alias = "OffsetEXE")]
    exe_offset: i64,

    /// Size of setup.e32 after compression.
    #[doc(alias = "CompressedSizeEXE")]
    exe_compressed_size: u32,

    /// Size of setup.e32 before compression.
    #[doc(alias = "UncompressedSizeEXE")]
    exe_uncompressed_size: u32,

    /// Checksum of setup.e32 before compression.
    #[doc(alias = "CRCEXE")]
    exe_checksum: Checksum,

    message_offset: u32,

    /// Offset of embedded setup-0.bin data.
    ///
    /// #### Revisions
    ///
    /// * 1: u32
    /// * 2: i64
    #[doc(alias = "Offset0")]
    header_offset: i64,

    /// Offset of embedded setup-1.bin data.
    ///
    /// #### Revisions
    ///
    /// * 1: u32
    /// * 2: i64
    #[doc(alias = "Offset1")]
    data_offset: i64,

    /// Reserved padding for future use, present in revision 2 and later.
    #[doc(alias = "ReservedPadding")]
    reserved_padding: u32,
}

impl SetupLoader {
    const EXE_MODE_OFFSET: u32 = 0x30;

    const TABLE_RESOURCE_ID: u32 = 11111;

    /// Attempts to find the setup loader via the legacy method, falling back to checking for a PE
    /// resource entry.
    pub fn read_from<R>(mut src: R) -> InnoResult<Self>
    where
        R: Read + Seek,
    {
        Self::read_legacy(&mut src).or_else(|_| Self::read_from_resource(&mut src))
    }

    /// Prior to Inno 5.1.5, the offset table is found by following a pointer at a constant offset.
    fn read_legacy<R>(mut reader: R) -> InnoResult<Self>
    where
        R: Read + Seek,
    {
        // Seek to the setup loader offset header
        reader.seek(SeekFrom::Start(Self::EXE_MODE_OFFSET.into()))?;

        // Read the setup loader offset header
        let setup_loader_offset = SetupLoaderOffset::try_read(&mut reader)?;

        // Seek to the setup loader offset table
        reader.seek(SeekFrom::Start(setup_loader_offset.table_offset().into()))?;

        Self::new(reader)
    }

    /// Since Inno 5.1.5, the offset table is stored as a PE resource entry
    fn read_from_resource<R>(mut reader: R) -> InnoResult<Self>
    where
        R: Read + Seek,
    {
        // Seek to the start of the file
        reader.seek(SeekFrom::Start(0))?;

        // Read DOS Header
        let dos_header = DosHeader::try_read_from_io(&mut reader)?;

        // Seek to PE header
        reader.seek(SeekFrom::Start(dos_header.pe_pointer().into()))?;

        // Read PE Signature
        let _signature = Signature::try_read_from_io(&mut reader)?; // PE/0/0

        // Read COFF header
        let coff_header = reader.read_t::<CoffHeader>()?;

        // Read optional header
        let optional_header = OptionalHeader::read_from(&mut reader)?;

        // Read the section table
        let section_table = SectionTable::read_from(&mut reader, coff_header)?;

        // Get the resource table data directory header
        let resource_table = optional_header
            .data_directories
            .resource_table()
            .ok_or_else(|| {
                InnoError::Io(io::Error::new(
                    io::ErrorKind::InvalidData,
                    "No resource table found in executable",
                ))
            })?;

        // Get the actual file offset of the resource directory section
        let resource_directory_offset = resource_table.file_offset(&section_table)?;

        let section_reader = SectionReader::new(
            &mut reader,
            resource_directory_offset.into(),
            resource_table.size().into(),
        )?;

        let mut resource_directory = ResourceDirectory::new(section_reader)?;

        let _rc_data = resource_directory.find_rc_data()?;

        let loader_directory_table =
            resource_directory.find_directory_table_by_id(Self::TABLE_RESOURCE_ID)?;

        let loader_directory = loader_directory_table.entries().next().ok_or_else(|| {
            InnoError::Io(io::Error::new(
                io::ErrorKind::InvalidData,
                "No loader data entry found in loader directory",
            ))
        })?;

        let loader_data_entry_offset = loader_directory.file_offset(resource_directory_offset);
        reader.seek(SeekFrom::Start(loader_data_entry_offset.into()))?;

        let loader_data_entry = reader.read_t::<ImageResourceDataEntry>()?;
        let setup_loader_offset =
            section_table.to_file_offset(loader_data_entry.offset_to_data())?;

        reader.seek(SeekFrom::Start(setup_loader_offset.into()))?;

        Self::new(reader.take(loader_data_entry.size().into()))
    }

    fn new<R>(reader: R) -> InnoResult<Self>
    where
        R: Read,
    {
        let mut checksum = Crc32Reader::new(reader);

        let signature = SetupLoaderSignature::read_from(&mut checksum)?;

        let loader_version = signature
            .version()
            .ok_or(InnoError::UnknownLoaderSignature(signature.as_array()))?;

        let revision = if loader_version >= (5, 1, 5) {
            checksum.read_u32::<LE>()?
        } else {
            0
        };

        let minimum_setup_exe_size = if revision >= 2 {
            checksum.read_i64::<LE>()?
        } else {
            checksum.read_u32::<LE>()?.into()
        };

        let exe_offset = if revision >= 2 {
            checksum.read_i64::<LE>()?
        } else {
            checksum.read_u32::<LE>()?.into()
        };

        let exe_compressed_size = if loader_version >= (4, 1, 6) {
            0
        } else {
            checksum.read_u32::<LE>()?
        };

        let exe_uncompressed_size = checksum.read_u32::<LE>()?;

        let exe_checksum = if loader_version >= (4, 0, 3) {
            Checksum::Crc32(checksum.read_u32::<LE>()?)
        } else {
            Checksum::Adler32(checksum.read_u32::<LE>()?)
        };

        let message_offset = if loader_version >= 4 {
            0
        } else {
            checksum.get_mut().read_u32::<LE>()?
        };

        let header_offset = if revision >= 2 {
            checksum.read_i64::<LE>()?
        } else {
            checksum.read_u32::<LE>()?.into()
        };

        let data_offset = if revision >= 2 {
            checksum.read_i64::<LE>()?
        } else {
            checksum.read_u32::<LE>()?.into()
        };

        let reserved_padding = if revision >= 2 {
            checksum.read_u32::<LE>()?
        } else {
            0
        };

        if loader_version >= (4, 0, 10) {
            let expected_checksum = checksum.get_mut().read_u32::<LE>()?;
            let actual_checksum = checksum.finalize();
            if actual_checksum != expected_checksum {
                return Err(InnoError::ChecksumMismatch {
                    location: "Setup Loader",
                    inner: ChecksumMismatchError::new_crc32(expected_checksum, actual_checksum),
                });
            }
        }

        Ok(Self {
            signature,
            version: loader_version,
            revision,
            minimum_setup_exe_size,
            exe_offset,
            exe_compressed_size,
            exe_uncompressed_size,
            exe_checksum,
            message_offset,
            header_offset,
            data_offset,
            reserved_padding,
        })
    }

    /// Returns the version of the setup loader.
    #[must_use]
    #[inline]
    pub const fn version(&self) -> InnoVersion {
        self.version
    }

    /// Returns the revision number of the setup loader.
    #[doc(alias = "SetupLdrOffsetTableVersion")]
    #[must_use]
    #[inline]
    pub const fn revision(&self) -> u32 {
        self.revision
    }

    /// Returns the signature of the setup loader.
    #[doc(alias = "ID")]
    #[must_use]
    #[inline]
    pub const fn signature(&self) -> SetupLoaderSignature {
        self.signature
    }

    /// Returns the minimum expected size of the setup.exe file.
    #[doc(alias = "TotalSize")]
    #[must_use]
    #[inline]
    pub const fn minimum_setup_exe_size(&self) -> i64 {
        self.minimum_setup_exe_size
    }

    /// Returns the offset of the compressed setup.e32 file.
    #[doc(alias = "OffsetEXE")]
    #[must_use]
    #[inline]
    pub const fn exe_offset(&self) -> i64 {
        self.exe_offset
    }

    /// Returns the size of the compressed setup.e32 file.
    #[doc(alias = "CompressedSizeEXE")]
    #[must_use]
    #[inline]
    pub const fn exe_compressed_size(&self) -> u32 {
        self.exe_compressed_size
    }

    /// Returns the size of the uncompressed setup.e32 file.
    #[doc(alias = "UncompressedSizeEXE")]
    #[must_use]
    #[inline]
    pub const fn exe_uncompressed_size(&self) -> u32 {
        self.exe_uncompressed_size
    }

    /// Returns the checksum of the uncompressed setup.e32 file.
    #[doc(alias = "CRCEXE")]
    #[must_use]
    #[inline]
    pub const fn exe_checksum(&self) -> Checksum {
        self.exe_checksum
    }

    /// Returns the offset of the message resource.
    #[must_use]
    #[inline]
    pub const fn message_offset(&self) -> u32 {
        self.message_offset
    }

    /// Returns the offset of the embedded setup-0.bin data.
    #[doc(alias = "Offset0")]
    #[must_use]
    #[inline]
    pub const fn header_offset(&self) -> i64 {
        self.header_offset
    }

    /// Returns the offset of the embedded setup-1.bin data.
    #[doc(alias = "Offset1")]
    #[must_use]
    #[inline]
    pub const fn data_offset(&self) -> i64 {
        self.data_offset
    }
}

#[cfg(test)]
mod tests {
    use super::SetupLoader;

    #[test]
    fn size() {
        assert_eq!(size_of::<SetupLoader>(), 80);
    }
}
