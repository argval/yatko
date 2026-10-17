mod compression_filter;
mod file;
mod flags;

pub(super) mod instruction;
mod sign;

use std::io;

#[cfg(feature = "chrono")]
use chrono::{DateTime, Utc};
pub use compression_filter::CompressionFilter;
pub use file::File;
pub use flags::FileLocationFlags;
#[cfg(feature = "jiff")]
use jiff::Timestamp;
use nt_time::{FileTime, dos_date_time};
pub use sign::SignMode;
use zerocopy::LE;

use super::checksum::Checksum;
use crate::{
    header::{Compression, Header, flag_reader::read_flags::read_flags},
    read::{
        ReadBytesExt,
        chunk::{Chunk, Encryption},
    },
    version::InnoVersion,
};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct FileLocation {
    chunk: Chunk,
    file: File,
    uncompressed_size: u64,
    file_time: FileTime,
    file_version: u64,
    options: FileLocationFlags,
    sign_mode: SignMode,
}

impl FileLocation {
    pub fn read<R>(mut reader: R, header: &Header, version: InnoVersion) -> io::Result<Self>
    where
        R: io::Read,
    {
        let mut chunk = Chunk {
            first_slice: reader.read_u32::<LE>()?,
            last_slice: reader.read_u32::<LE>()?,
            ..Chunk::default()
        };

        if version < 4 && chunk.first_slice >= 1 && chunk.last_slice >= 1 {
            chunk.first_slice -= 1;
            chunk.last_slice -= 1;
        }

        chunk.sub_offset = if version >= (6, 5, 2) {
            reader.read_u64::<LE>()?
        } else {
            reader.read_u32::<LE>()?.into()
        };
        chunk.start_offset = chunk.sub_offset;

        let mut file = File::default();

        if version >= (4, 0, 1) {
            file.offset = reader.read_u64::<LE>()?;
        }

        if version >= 4 {
            file.size = reader.read_u64::<LE>()?;
            chunk.original_size = reader.read_u64::<LE>()?;
        } else {
            file.size = reader.read_u32::<LE>()?.into();
            chunk.original_size = reader.read_u32::<LE>()?.into();
        }

        let uncompressed_size = file.size;

        file.checksum = if version >= 6.4 {
            Checksum::read_sha256(&mut reader)?
        } else if version >= (5, 3, 9) {
            Checksum::read_sha1(&mut reader)?
        } else if version >= 4.2 {
            Checksum::read_md5(&mut reader)?
        } else if version >= (4, 0, 1) {
            Checksum::read_crc32(&mut reader)?
        } else {
            Checksum::read_adler32(&mut reader)?
        };

        let file_time = if version.is_16_bit() {
            // 16-bit installers use the FAT filetime format
            use dos_date_time::{Date, DateTime, Time};

            let time = Time::new(reader.read_u16::<LE>()?).unwrap_or_default();
            let date = Date::new(reader.read_u16::<LE>()?).unwrap_or_default();

            DateTime::new(date, time).into()
        } else {
            // 32-bit installers use the Win32 FILETIME format
            FileTime::new(reader.read_u64::<LE>()?)
        };

        let file_version_ms = reader.read_u32::<LE>()?;
        let file_version_ls = reader.read_u32::<LE>()?;
        let file_version =
            u64::from(file_version_ms) << u64::from(u32::BITS) | u64::from(file_version_ls);

        let mut options = read_flags!(&mut reader,
            FileLocationFlags::VERSION_INFO_VALID,
            if version < (6, 4, 3) => FileLocationFlags::VERSION_INFO_NOT_VALID,
            if ((2, 0, 17)..(4, 0, 1)).contains(&version) => FileLocationFlags::BZIPPED,
            if version >= (4, 0, 10) => FileLocationFlags::TIMESTAMP_IN_UTC,
            if ((4, 2, 0)..(6, 4, 3)).contains(&version) => FileLocationFlags::IS_UNINSTALLER_EXE,
            if version >= (4, 1, 8) => FileLocationFlags::CALL_INSTRUCTION_OPTIMIZED,
            if ((4, 2, 0)..(6, 4, 3)).contains(&version) => FileLocationFlags::TOUCH,
            if version >= (4, 2, 2) => FileLocationFlags::CHUNK_ENCRYPTED,
            if version >= (4, 2, 5) => FileLocationFlags::CHUNK_COMPRESSED,
            if ((5, 1, 13)..(6, 4, 3)).contains(&version) => FileLocationFlags::SOLID_BREAK,
            if version >= (5, 5, 7) && version < 6.3 => [
                FileLocationFlags::SIGN,
                FileLocationFlags::SIGN_ONCE
            ]
        )?;

        if version < (4, 2, 5) {
            options |= FileLocationFlags::CHUNK_COMPRESSED;
        }

        let sign_mode = if ((6, 3, 0)..(6, 4, 3)).contains(&version) {
            SignMode::try_read_from_io(&mut reader)?
        } else {
            SignMode::from(options)
        };

        if options.contains(FileLocationFlags::CHUNK_COMPRESSED) {
            chunk.compression = header.compression();
        } else {
            chunk.compression = Compression::Stored;
        }

        if options.contains(FileLocationFlags::BZIPPED) {
            options |= FileLocationFlags::CHUNK_COMPRESSED;
            chunk.compression = Compression::BZip2;
        }

        chunk.encryption = if options.contains(FileLocationFlags::CHUNK_ENCRYPTED) {
            if version >= 6.4 {
                Encryption::XChaCha20
            } else if version >= (5, 3, 9) {
                Encryption::Arc4Sha1
            } else {
                Encryption::Arc4Md5
            }
        } else {
            Encryption::Plaintext
        };

        file.compression_filter = if options.contains(FileLocationFlags::CALL_INSTRUCTION_OPTIMIZED)
        {
            if version < 5.2 {
                CompressionFilter::InstructionFilter4108
            } else if version < (5, 3, 9) {
                CompressionFilter::InstructionFilter5200
            } else {
                CompressionFilter::InstructionFilter5309
            }
        } else {
            CompressionFilter::NoFilter
        };

        Ok(Self {
            chunk,
            file,
            uncompressed_size,
            file_time,
            file_version,
            options,
            sign_mode,
        })
    }

    /// Returns the chunk.
    #[must_use]
    #[inline]
    pub const fn chunk(&self) -> &Chunk {
        &self.chunk
    }

    /// Returns the file object.
    #[must_use]
    #[inline]
    pub const fn file(&self) -> File {
        self.file
    }

    /// Returns the uncompressed size.
    #[must_use]
    #[inline]
    pub const fn uncompressed_size(&self) -> u64 {
        self.uncompressed_size
    }

    /// Returns the file's last modified filetime.
    #[must_use]
    #[inline]
    pub const fn file_time(&self) -> FileTime {
        self.file_time
    }

    /// Returns the file's created at time as a [`DateTime<Utc>`].
    #[cfg(feature = "chrono")]
    #[must_use]
    #[inline]
    pub fn date_time(&self) -> DateTime<Utc> {
        self.file_time.into()
    }

    /// Returns the file's created at time as a [`Timestamp`].
    ///
    /// # Errors
    ///
    /// Returns [`Err`] if the time is out of range for [`Timestamp`].
    #[cfg(feature = "jiff")]
    #[inline]
    pub fn timestamp(&self) -> Result<Timestamp, jiff::Error> {
        self.file_time.try_into()
    }

    /// Returns the file version.
    #[must_use]
    #[inline]
    pub const fn file_version(&self) -> u64 {
        self.file_version
    }

    /// Returns the file option flags.
    #[must_use]
    #[inline]
    pub const fn file_option_flags(&self) -> FileLocationFlags {
        self.options
    }

    /// Returns the sign mode.
    #[must_use]
    #[inline]
    pub const fn sign_mode(&self) -> SignMode {
        self.sign_mode
    }
}
