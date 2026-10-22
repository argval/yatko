//! The `.bin` files an installer's data is split across.
//!
//! An installer built to fit on removable media, or simply built with a size
//! limit, keeps its header in the setup executable and its data in numbered
//! slices beside it:
//!
//! ```text
//! setup_game.exe      the header, and nothing else
//! setup_game-1.bin    slice 0
//! setup_game-2.bin    slice 1
//! ```
//!
//! Each file entry says which slice its data starts in, and the offsets it
//! records are positions within that file, counted from its beginning rather
//! than from the end of its header. An installer that keeps its data in the
//! executable instead records a non-zero data offset and puts everything in
//! slice 0, which is why one rule covers both: seek to the data offset plus
//! the recorded offset, in the file for that slice.

use std::{
    fs::File,
    io::{self, BufReader, Read, Seek, SeekFrom},
    num::NonZeroU8,
    path::{Path, PathBuf},
};

use zerocopy::LE;

use crate::read::{DataSource, ReadBytesExt};

/// The first bytes of every slice file. The two spellings are the 16-bit and
/// 32-bit builds of Inno Setup, and nothing else about them differs.
const MAGIC: [&[u8; 8]; 2] = [b"idska16\x1a", b"idska32\x1a"];

/// What a slice file's own header takes up: the magic and the size that
/// follows it. Offsets recorded in the installer count from the start of the
/// file, so this is where a slice's first chunk can be.
const HEADER_SIZE: u64 = 12;

/// How many slice files one disk is split into. Inno Setup's `SlicesPerDisk`
/// directive takes 1 to 26, the range a single letter can name.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) struct SlicesPerDisk(NonZeroU8);

impl SlicesPerDisk {
    /// The largest the directive allows: with 26 files on a disk the last is
    /// named `z`, and a 27th would need a second letter.
    pub const MAX: u32 = 26;

    #[must_use]
    pub const fn get(self) -> u32 {
        self.0.get() as u32
    }

    /// The disk `slice` belongs to, numbered from one.
    const fn disk(self, slice: u32) -> u32 {
        (slice / self.get()) + 1
    }

    /// Which of that disk's files `slice` is, as the letter the name uses:
    /// `a` for the first on the disk, `b` for the second. Total because the
    /// invariant holds, so the remainder never leaves `a`..=`z`.
    const fn letter(self, slice: u32) -> char {
        (b'a' + (slice % self.get()) as u8) as char
    }
}

impl Default for SlicesPerDisk {
    fn default() -> Self {
        Self(NonZeroU8::MIN)
    }
}

impl TryFrom<u32> for SlicesPerDisk {
    type Error = u32;

    /// Fails with the value itself, for an error that can name it.
    fn try_from(value: u32) -> Result<Self, Self::Error> {
        u8::try_from(value)
            .ok()
            .filter(|&narrowed| u32::from(narrowed) <= Self::MAX)
            .and_then(NonZeroU8::new)
            .map(Self)
            .ok_or(value)
    }
}

/// Returns the name of the file holding `slice`.
///
/// With one slice per disk they are numbered from one, so slice 0 is
/// `<base>-1.bin`. With more, each disk is a number and each slice on it a
/// letter, giving `<base>-1a.bin`, `<base>-1b.bin`, and so on.
#[must_use]
fn filename(base: &str, slice: u32, slices_per_disk: SlicesPerDisk) -> String {
    if slices_per_disk.get() == 1 {
        return format!("{base}-{}.bin", slice + 1);
    }

    format!(
        "{base}-{}{}.bin",
        slices_per_disk.disk(slice),
        slices_per_disk.letter(slice)
    )
}

/// Reads and checks a slice file's own header, returning the size it claims.
fn read_header(reader: &mut impl Read, name: &str) -> io::Result<u64> {
    let mut magic = [0_u8; 8];
    reader.read_exact(&mut magic)?;

    if !MAGIC.contains(&&magic) {
        return Err(io::Error::new(
            io::ErrorKind::InvalidData,
            format!("{name} does not begin like a slice of an installer"),
        ));
    }

    Ok(u64::from(reader.read_u32::<LE>()?))
}

/// The slice files of one installer, opened as they are needed.
///
/// Only one is held open at a time. Extraction walks the data in order, so a
/// slice is finished with before the next is wanted, and an installer split
/// across a dozen files does not need a dozen open descriptors to read one.
pub(crate) struct Slices {
    directory: PathBuf,
    base: String,
    /// A second name to try, from the installer's own header, for a file that
    /// has been renamed since it was built.
    alternate: Option<String>,
    slices_per_disk: SlicesPerDisk,
    open: Option<Open>,
}

struct Open {
    slice: u32,
    size: u64,
    reader: BufReader<File>,
}

impl Slices {
    /// Prepares to read the slices beside `installer`, preferring `base` for
    /// their names and falling back to the installer's own stem.
    ///
    /// An installer records the base name its slices were built with, which
    /// is usually the same as the file's, and is not when the file has been
    /// renamed since. Trying both is what makes a renamed download work.
    #[must_use]
    pub fn beside_named(
        installer: &Path,
        base: Option<&str>,
        slices_per_disk: SlicesPerDisk,
    ) -> Self {
        let mut slices = Self::beside(installer, slices_per_disk);
        // The base name comes out of the installer's own header and is used
        // to build a filename beside it, so it must not be a path.
        slices.alternate = base
            .map(|base| base.replace(['/', '\\'], "_"))
            .filter(|base| !base.is_empty() && *base != slices.base);
        slices
    }

    /// Prepares to read the slices beside `installer`, which is the path of
    /// the setup executable.
    ///
    /// Nothing is opened here: an installer whose data is in the executable
    /// has no slices at all, and finding that out should not be an error
    /// until something actually asks for data.
    #[must_use]
    pub fn beside(installer: &Path, slices_per_disk: SlicesPerDisk) -> Self {
        Self {
            directory: installer
                .parent()
                .unwrap_or_else(|| Path::new("."))
                .to_path_buf(),
            base: installer
                .file_stem()
                .unwrap_or_default()
                .to_string_lossy()
                .into_owned(),
            alternate: None,
            slices_per_disk,
            open: None,
        }
    }

    /// Returns the path the given slice is expected at, which is the
    /// alternate name's only if the preferred one is not there.
    #[must_use]
    pub fn path(&self, slice: u32) -> PathBuf {
        let preferred = self
            .directory
            .join(filename(&self.base, slice, self.slices_per_disk));

        if preferred.exists() {
            return preferred;
        }

        match &self.alternate {
            Some(alternate) => {
                let other = self
                    .directory
                    .join(filename(alternate, slice, self.slices_per_disk));
                if other.exists() { other } else { preferred }
            }
            // Named as expected, not as stored.
            None => preferred,
        }
    }

    /// Makes `slice` the open one, leaving the reader positioned just past
    /// its header. Already open, nothing happens and the position is kept.
    fn open(&mut self, slice: u32) -> io::Result<&mut Open> {
        if self.open.as_ref().is_none_or(|open| open.slice != slice) {
            let path = self.path(slice);
            let mut reader = BufReader::new(File::open(&path).map_err(|error| {
                io::Error::new(error.kind(), format!("{}: {error}", path.display()))
            })?);
            let size = read_header(&mut reader, &path.to_string_lossy())?;

            self.open = Some(Open {
                slice,
                size,
                reader,
            });
        }

        Ok(self.open.as_mut().expect("just opened"))
    }
}

impl DataSource for Slices {
    fn seek_to(&mut self, slice: u32, offset: u64) -> io::Result<()> {
        let open = self.open(slice)?;

        if offset > open.size {
            return Err(io::Error::new(
                io::ErrorKind::InvalidInput,
                format!(
                    "offset {offset} is past the end of slice {slice}, which is {} bytes",
                    open.size
                ),
            ));
        }

        open.reader.seek(SeekFrom::Start(offset))?;
        Ok(())
    }
}

impl Read for Slices {
    fn read(&mut self, buffer: &mut [u8]) -> io::Result<usize> {
        let Some(open) = self.open.as_mut() else {
            return Err(io::Error::new(
                io::ErrorKind::NotConnected,
                "no slice has been sought to",
            ));
        };

        let position = open.reader.stream_position()?;
        if position < open.size {
            let remaining = usize::try_from(open.size - position).unwrap_or(usize::MAX);
            let wanted = buffer.len().min(remaining);
            return open.reader.read(&mut buffer[..wanted]);
        }

        // The slice is spent. A chunk is allowed to run on into the next one,
        // where it continues immediately after that slice's own header.
        let next = open.slice + 1;
        let open = match self.open(next) {
            Ok(open) => open,
            // No next file is the end of the data, not a failure to read it.
            // As an error it would break every reader that finishes by asking
            // once more. A chunk genuinely cut short still fails, in the layer
            // that knows how much it was owed.
            Err(error) if error.kind() == io::ErrorKind::NotFound => return Ok(0),
            Err(error) => return Err(error),
        };
        open.reader.seek(SeekFrom::Start(HEADER_SIZE))?;
        let remaining =
            usize::try_from(open.size.saturating_sub(HEADER_SIZE)).unwrap_or(usize::MAX);
        let wanted = buffer.len().min(remaining);
        open.reader.read(&mut buffer[..wanted])
    }
}

#[cfg(test)]
mod tests {
    fn per_disk(value: u32) -> SlicesPerDisk {
        SlicesPerDisk::try_from(value).unwrap()
    }

    #[test]
    fn the_slices_per_disk_range_is_the_one_a_single_letter_can_name() {
        assert!(SlicesPerDisk::try_from(0).is_err());
        assert!(SlicesPerDisk::try_from(1).is_ok());
        assert!(SlicesPerDisk::try_from(SlicesPerDisk::MAX).is_ok());
        assert_eq!(SlicesPerDisk::try_from(SlicesPerDisk::MAX + 1), Err(27));
    }

    /// A value is rejected on what the header says, not on its low byte. 257
    /// and 282 are the ends of the range that folds onto 1 and 26.
    #[test]
    fn a_value_past_the_range_is_not_wrapped_into_it() {
        for value in [
            256,
            257,
            282,
            283,
            512,
            513,
            538,
            u32::from(u8::MAX) + 1 + SlicesPerDisk::MAX,
            u32::MAX,
        ] {
            assert_eq!(SlicesPerDisk::try_from(value), Err(value));
        }
    }

    #[test]
    fn the_last_slice_on_a_full_disk_is_named_z() {
        let last = SlicesPerDisk::MAX - 1;
        assert_eq!(
            filename("game", last, per_disk(SlicesPerDisk::MAX)),
            "game-1z.bin"
        );
        assert_eq!(
            filename("game", last + 1, per_disk(SlicesPerDisk::MAX)),
            "game-2a.bin"
        );
    }

    use std::io::{Read, Write};

    use super::{HEADER_SIZE, Slices, SlicesPerDisk, filename, read_header};
    use crate::read::DataSource;

    /// Writes a slice file holding `content` after its header, and returns
    /// the directory it is in along with the installer path it sits beside.
    fn slice_on_disk(content: &[u8], slice: u32) -> (tempfile::TempDir, std::path::PathBuf) {
        let directory = tempfile::tempdir().expect("temp dir");
        let installer = directory.path().join("setup_game.exe");

        let mut file = std::fs::File::create(directory.path().join(filename(
            "setup_game",
            slice,
            per_disk(1),
        )))
        .expect("create slice");
        file.write_all(b"idska32\x1a").expect("magic");
        let size = HEADER_SIZE + content.len() as u64;
        file.write_all(&(size as u32).to_le_bytes()).expect("size");
        file.write_all(content).expect("content");

        (directory, installer)
    }

    #[test]
    fn data_is_read_from_the_offset_the_installer_records() {
        // Those offsets count from the start of the file, header included,
        // so the first chunk of a slice sits at exactly HEADER_SIZE.
        let (_directory, installer) = slice_on_disk(b"the payload", 0);
        let mut slices = Slices::beside(&installer, per_disk(1));

        slices.seek_to(0, HEADER_SIZE).expect("seek");
        let mut out = String::new();
        slices.read_to_string(&mut out).expect("read");
        assert_eq!(out, "the payload");
    }

    #[test]
    fn reading_stops_at_the_end_of_the_slice_rather_than_at_the_end_of_the_file() {
        // A slice file can be longer than the size its header claims. Reading
        // to the end of the file would serve whatever follows as though it
        // were installer data.
        let (directory, installer) = slice_on_disk(b"real", 0);
        let mut file = std::fs::OpenOptions::new()
            .append(true)
            .open(directory.path().join("setup_game-1.bin"))
            .expect("reopen");
        file.write_all(b"trailing junk").expect("append");

        let mut slices = Slices::beside(&installer, per_disk(1));
        slices.seek_to(0, HEADER_SIZE).expect("seek");

        let mut out = Vec::new();
        slices.read_to_end(&mut out).expect("read");
        assert_eq!(out, b"real");
    }

    #[test]
    fn a_missing_slice_is_reported_with_the_name_that_was_looked_for() {
        // The person has a file they downloaded and a file we cannot find;
        // saying which one is missing is the whole of the error's job.
        let directory = tempfile::tempdir().expect("temp dir");
        let installer = directory.path().join("setup_game.exe");
        let mut slices = Slices::beside(&installer, per_disk(1));

        let error = slices.seek_to(0, HEADER_SIZE).unwrap_err();
        assert!(error.to_string().contains("setup_game-1.bin"), "{error}");
    }

    #[test]
    fn the_name_out_of_the_header_is_used_when_the_file_has_been_renamed() {
        let (directory, _) = slice_on_disk(b"payload", 0);
        let renamed = directory.path().join("downloaded (1).exe");
        let mut slices = Slices::beside_named(&renamed, Some("setup_game"), per_disk(1));

        slices.seek_to(0, HEADER_SIZE).expect("seek");
        let mut out = String::new();
        slices.read_to_string(&mut out).expect("read");
        assert_eq!(out, "payload");
    }

    #[test]
    fn a_name_out_of_the_header_cannot_reach_another_directory() {
        // It comes out of the installer, so it is not to be trusted with a
        // path separator.
        let directory = tempfile::tempdir().expect("temp dir");
        let installer = directory.path().join("setup_game.exe");
        let slices = Slices::beside_named(&installer, Some("../../etc/passwd"), per_disk(1));

        let path = slices.path(0);
        assert!(!path.to_string_lossy().contains(".."), "{}", path.display());
    }

    #[test]
    fn one_slice_per_disk_counts_from_one() {
        // Slice 0 is `-1.bin`, which is the off-by-one every reader of these
        // files has to get right: the installer counts from zero and the
        // filenames from one.
        assert_eq!(filename("setup_game", 0, per_disk(1)), "setup_game-1.bin");
        assert_eq!(filename("setup_game", 1, per_disk(1)), "setup_game-2.bin");
        assert_eq!(filename("setup_game", 9, per_disk(1)), "setup_game-10.bin");
    }

    #[test]
    fn several_slices_per_disk_are_lettered_within_a_numbered_disk() {
        assert_eq!(filename("game", 0, per_disk(3)), "game-1a.bin");
        assert_eq!(filename("game", 2, per_disk(3)), "game-1c.bin");
        assert_eq!(filename("game", 3, per_disk(3)), "game-2a.bin");
        assert_eq!(filename("game", 5, per_disk(3)), "game-2c.bin");
    }

    #[test]
    fn a_name_with_its_own_punctuation_is_left_alone() {
        // GOG names its installers like this, and the stem is used as given.
        assert_eq!(
            filename("setup_rollercoaster_tycoon_2_(76932)", 0, per_disk(1)),
            "setup_rollercoaster_tycoon_2_(76932)-1.bin"
        );
    }

    #[test]
    fn a_slice_header_gives_up_the_size_it_claims() {
        let mut data = b"idska32\x1a".to_vec();
        data.extend_from_slice(&1234_u32.to_le_bytes());
        assert_eq!(read_header(&mut data.as_slice(), "test").unwrap(), 1234);
    }

    #[test]
    fn the_sixteen_bit_spelling_of_the_magic_is_also_a_slice() {
        let mut data = b"idska16\x1a".to_vec();
        data.extend_from_slice(&7_u32.to_le_bytes());
        assert_eq!(read_header(&mut data.as_slice(), "test").unwrap(), 7);
    }

    #[test]
    fn something_that_is_not_a_slice_is_refused_by_name() {
        // Pointing this at the setup executable instead of a .bin is an easy
        // mistake, and the error has to say which file was wrong.
        let mut data = b"MZ\x90\x00\x03\x00\x00\x00".to_vec();
        data.extend_from_slice(&0_u32.to_le_bytes());
        let error = read_header(&mut data.as_slice(), "setup_game.exe").unwrap_err();
        assert!(error.to_string().contains("setup_game.exe"), "{error}");
    }

    #[test]
    fn a_file_too_short_to_hold_a_header_is_an_error() {
        assert!(read_header(&mut b"idska32".as_slice(), "short.bin").is_err());
    }
}
