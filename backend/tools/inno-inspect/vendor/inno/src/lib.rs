#![forbid(unsafe_code)]

/*!
Inno is a read-only parser for [Inno Setup](https://jrsoftware.org/isinfo.php) installers (.exe).
It reads the installer structures so you can inspect an installer without actually running it.

This crate focuses on correctness across a wide range of Inno Setup versions and provides
strongly-typed access to sections like Languages, Files, Tasks, Components, Registry entries, and
more. It does not execute any installer logic.

# Getting started

Add this to your `Cargo.toml`:

```toml
[dependencies]
inno = "0.7"
```

Then, open an installer and inspect its contents:

```no_run
use std::fs::File;
use inno::{Inno, error::InnoResult};

fn main() -> InnoResult<()> {
    let file = File::open("path/to/setup.exe")?;
    let inno = Inno::new(file)?;

    println!("Inno Setup version: {}", inno.version());

    if let Some(name) = inno.header().app_name() {
        println!("App name: {name}");
    }

    println!("Languages ({}):", inno.languages().len());
    for lang in inno.languages() {
        println!("- {}", lang.name());
    }

    Ok(())
}
```

## Optional Features

The following are a list of [Cargo features][cargo-features] that can be enabled or disabled:

- **chrono**: Enables converting a file's created at time to a [`DateTime<UTC>`].
- **jiff**: Enables converting a file's created at time to a [`Timestamp`].
- **extract**: Enables extracting file contents and verifying their checksums.

# What this crate provides

- A high-level [`Inno`] struct representing a parsed installer, including:
  - [`Inno::version`]: the detected installer version and variant (Unicode/ANSI, ISX, etc.).
  - [`Inno::header`]: top-level metadata and configuration (app name, publisher, compression, wizard
    settings, etc.).
  - Collections for languages, messages, permissions, types, components, tasks, directories, files,
    icons, INI entries, registry entries, and run entries.
  - [`Inno::file_locations`]: low-level data/file location entries for consumers that want to
    implement extraction.
- Safe decoding of text according to the installer’s Unicode/ANSI mode and language codepage.
- Version-aware parsing that accounts for structural changes between Inno Setup releases.

See the `innex` CLI in this repository for a reference consumer.

# Supported versions

Parsing is supported up to Inno Setup 7.1.x. Newer installers may work but can introduce format
changes. In that case, you will get [`InnoError::UnsupportedVersion`].

# Features

- LZMA/BZip2/Deflate detection through header metadata.
- LZMA1 decompression of the Inno header stream via the pure-Rust `lzma-rust2`
  crate, with no C dependency or native toolchain required.

# Error handling

All fallible operations return [`InnoError`]. Typical errors include:

- Not an Inno installer file.
- Unsupported (too-new) installer version.
- I/O errors while reading.
- Unexpected data at the end of a header stream (corruption or truncated file).

# Notes on text decoding

In Unicode installers, text is always read as UTF-16LE. In ANSI installers,
this crate picks a codepage based on the language table, preferring
Windows-1252 when no explicit match is found, to maximize compatibility with
older installers.

# Minimum Supported Rust Version (MSRV)

This crate is tested with Rust 1.88 or newer. Newer Rust versions are generally recommended.

# Acknowledgements

- innoextract: <https://github.com/dscharrer/innoextract>
- Inno Setup: <https://jrsoftware.org/isinfo.php>

[cargo-features]: https://doc.rust-lang.org/stable/cargo/reference/manifest.html#the-features-section
[`DateTime<Utc>`]: https://docs.rs/chrono/latest/chrono/struct.DateTime.html
[`Timestamp`]: https://docs.rs/jiff/latest/jiff/struct.Timestamp.html
*/

#![doc(html_root_url = "https://docs.rs/inno")]
#![allow(dead_code)]

mod compression;
mod encryption;
pub mod entry;
pub mod error;
pub mod header;
#[cfg(feature = "extract")]
mod iterator;
mod loader;
mod lzma_stream_header;
mod pe;
mod read;
mod slice;
pub mod string;
pub mod version;
mod wizard;

use std::{
    fs, io,
    io::{Read, Seek, SeekFrom},
    path::Path,
};

use encoding_rs::{UTF_16LE, WINDOWS_1252};
use encryption::EncryptionHeader;
use entry::{
    Component, DeleteEntry, Directory, File, FileLocation, ISSigKey, Icon, Ini, Language, Message,
    MessageEntry, Permission, RegistryEntry, RunEntry, Task, Type,
};
use error::InnoError;
pub use error::InnoResult;
pub use header::Header;
#[cfg(feature = "extract")]
use iterator::{ExtractEntry, FilesIterator, FilteredFilesIterator};
#[cfg(feature = "extract")]
pub use iterator::{FileReader, StreamingFiles};
use itertools::Itertools;
use loader::SetupLoader;
use lzma_stream_header::LzmaStreamHeader;
use read::{
    ReadBytesExt,
    stream::{InnoStreamReader, Primary, Secondary},
};
use version::{InnoVersion, windows_version::WindowsVersionRange};
pub use wizard::Wizard;
pub use zerocopy;

/// The maximum supported Inno Version by this library.
///
/// Inno Setup versions newer than this version are likely to have breaking changes where the
/// changes have not yet been implemented into this library.
pub const MAX_SUPPORTED_VERSION: InnoVersion = InnoVersion::new(7, 0, u8::MAX, u8::MAX);

#[derive(Debug)]
pub struct InnoInner {
    pub setup_loader: SetupLoader,
    version: InnoVersion,
    encryption_header: Option<EncryptionHeader>,
    pub header: Header,
    languages: Vec<Language>,
    messages: Vec<MessageEntry>,
    permissions: Vec<Permission>,
    type_entries: Vec<Type>,
    components: Vec<Component>,
    tasks: Vec<Task>,
    directories: Vec<Directory>,
    is_sig_keys: Vec<ISSigKey>,
    files: Vec<File>,
    icons: Vec<Icon>,
    ini_entries: Vec<Ini>,
    registry_entries: Vec<RegistryEntry>,
    delete_entries: Vec<DeleteEntry>,
    uninstall_delete_entries: Vec<DeleteEntry>,
    run_entries: Vec<RunEntry>,
    uninstall_run_entries: Vec<RunEntry>,
    wizard: Wizard,
    file_locations: Vec<FileLocation>,
}

impl InnoInner {
    fn read_stream<R: Read + Seek>(
        mut reader: R,
        setup_loader: SetupLoader,
        inno_version: InnoVersion,
    ) -> InnoResult<Self> {
        let encryption_header = if inno_version >= 6.5 {
            Some(EncryptionHeader::read(&mut reader, inno_version)?)
        } else {
            None
        };

        let mut reader = InnoStreamReader::new(&mut reader, inno_version)?;

        let mut header = Header::read(&mut reader, inno_version)?;

        let languages = (0..header.language_count())
            .map(|_| Language::read(&mut reader, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let codepage = if inno_version.is_unicode() {
            UTF_16LE
        } else {
            languages
                .iter()
                .map(Language::codepage)
                .find_or_first(|&codepage| codepage == WINDOWS_1252)
                .unwrap_or(WINDOWS_1252)
        };

        header.decode(codepage);

        let mut wizard = if inno_version < 4 {
            Wizard::read(&mut reader, &header, inno_version)?
        } else {
            Wizard::default()
        };

        let messages = (0..header.custom_message_count())
            .map(|_| MessageEntry::read(&mut reader, &languages, codepage))
            .collect::<io::Result<Vec<_>>>()?;

        let permissions = (0..header.permission_count())
            .map(|_| Permission::read(&mut reader))
            .collect::<io::Result<Vec<_>>>()?;

        let type_entries = (0..header.type_count())
            .map(|_| Type::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let components = (0..header.component_count())
            .map(|_| Component::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let tasks = (0..header.task_count())
            .map(|_| Task::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let directories = (0..header.directory_count())
            .map(|_| Directory::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let is_sig_keys = (0..header.is_sig_keys_count())
            .map(|_| ISSigKey::read(&mut reader, codepage))
            .collect::<io::Result<Vec<_>>>()?;

        let files = (0..header.file_count())
            .map(|_| File::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let icons = (0..header.icon_count())
            .map(|_| Icon::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let ini_entries = (0..header.ini_entry_count())
            .map(|_| Ini::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let registry_entries = (0..header.registry_entry_count())
            .map(|_| RegistryEntry::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let delete_entries = (0..header.install_delete_entry_count())
            .map(|_| DeleteEntry::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let uninstall_delete_entries = (0..header.uninstall_delete_entry_count())
            .map(|_| DeleteEntry::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let run_entries = (0..header.run_entry_count())
            .map(|_| RunEntry::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        let uninstall_run_entries = (0..header.uninstall_run_entry_count())
            .map(|_| RunEntry::read(&mut reader, codepage, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        if inno_version >= 4 {
            wizard = Wizard::read(&mut reader, &header, inno_version)?;
        }

        // Check that the reader is at the end of the primary header stream
        reader.check_end_of_stream::<Primary>()?;

        // Reset the block reader for the secondary header stream
        reader = reader.reset()?;

        let file_locations = (0..header.file_location_entry_count())
            .map(|_| FileLocation::read(&mut reader, &header, inno_version))
            .collect::<io::Result<Vec<_>>>()?;

        reader.check_end_of_stream::<Secondary>()?;

        Ok(Self {
            setup_loader,
            version: inno_version,
            encryption_header,
            header,
            languages,
            messages,
            permissions,
            type_entries,
            components,
            tasks,
            directories,
            is_sig_keys,
            files,
            icons,
            ini_entries,
            registry_entries,
            delete_entries,
            uninstall_delete_entries,
            run_entries,
            uninstall_run_entries,
            wizard,
            file_locations,
        })
    }

    #[inline]
    pub const fn setup_loader(&self) -> &SetupLoader {
        &self.setup_loader
    }

    pub const fn header(&self) -> &Header {
        &self.header
    }

    /// Returns the Inno Setup version.
    #[must_use]
    #[inline]
    pub const fn version(&self) -> InnoVersion {
        self.version
    }

    /// Returns the encryption header, if any.
    #[must_use]
    pub fn encryption_header(&self) -> Option<&EncryptionHeader> {
        self.encryption_header
            .as_ref()
            .or_else(|| self.header.encryption_header())
    }

    /// Returns the primary language of the installer, if available.
    #[must_use]
    #[inline]
    pub const fn primary_language(&self) -> Option<&Language> {
        self.languages().first()
    }

    /// Returns the languages as a slice.
    #[must_use]
    #[inline]
    pub const fn languages(&self) -> &[Language] {
        self.languages.as_slice()
    }

    /// Returns the message entries as a slice.
    #[must_use]
    #[inline]
    pub const fn message_entries(&self) -> &[MessageEntry] {
        self.messages.as_slice()
    }

    pub fn messages(&self) -> impl Iterator<Item = Message<'_, '_>> {
        self.messages
            .iter()
            .map(|message| Message::new(message, self.languages()))
    }

    /// Returns the permission entries as a slice.
    #[must_use]
    #[inline]
    pub const fn permissions(&self) -> &[Permission] {
        self.permissions.as_slice()
    }

    /// Returns the type entries as a slice.
    #[must_use]
    #[inline]
    pub const fn type_entries(&self) -> &[Type] {
        self.type_entries.as_slice()
    }

    /// Returns the component entries as a slice.
    #[must_use]
    #[inline]
    pub const fn components(&self) -> &[Component] {
        self.components.as_slice()
    }

    /// Returns the task entries as a slice.
    #[must_use]
    #[inline]
    pub const fn tasks(&self) -> &[Task] {
        self.tasks.as_slice()
    }

    /// Returns the directory entries as a slice.
    #[must_use]
    #[inline]
    pub const fn directories(&self) -> &[Directory] {
        self.directories.as_slice()
    }

    /// Returns the IS Sig Key entries as a slice.
    #[must_use]
    #[inline]
    pub const fn is_sig_keys(&self) -> &[ISSigKey] {
        self.is_sig_keys.as_slice()
    }

    /// Returns the file entries as a slice.
    #[must_use]
    #[inline]
    pub const fn file_entries(&self) -> &[File] {
        self.files.as_slice()
    }

    /// Returns the icon entries as a slice.
    #[must_use]
    #[inline]
    pub const fn icon_entries(&self) -> &[Icon] {
        self.icons.as_slice()
    }

    /// Returns the ini entries as a slice.
    #[must_use]
    #[inline]
    pub const fn ini_entries(&self) -> &[Ini] {
        self.ini_entries.as_slice()
    }

    /// Returns the registry entries a slice.
    #[must_use]
    #[inline]
    pub const fn registry_entries(&self) -> &[RegistryEntry] {
        self.registry_entries.as_slice()
    }

    /// Returns the delete entries as a slice.
    #[must_use]
    #[inline]
    pub const fn delete_entries(&self) -> &[DeleteEntry] {
        self.delete_entries.as_slice()
    }

    /// Returns the uninstall delete entries as a slice.
    #[must_use]
    #[inline]
    pub const fn uninstall_delete_entries(&self) -> &[DeleteEntry] {
        self.uninstall_delete_entries.as_slice()
    }

    /// Returns the run entries as a slice.
    #[must_use]
    #[inline]
    pub const fn run_entries(&self) -> &[RunEntry] {
        self.run_entries.as_slice()
    }

    /// Returns the uninstall run entries as a slice.
    #[must_use]
    #[inline]
    pub const fn uninstall_run_entries(&self) -> &[RunEntry] {
        self.uninstall_run_entries.as_slice()
    }

    /// Returns a reference to the [`Wizard`].
    #[must_use]
    #[inline]
    pub const fn wizard(&self) -> &Wizard {
        &self.wizard
    }

    /// Returns the file locations entries as a slice.
    #[must_use]
    #[inline]
    pub const fn file_locations(&self) -> &[FileLocation] {
        self.file_locations.as_slice()
    }
}

pub struct Inno<R: Read + Seek> {
    reader: R,
    /// The `.bin` files beside the installer, when its data is not in the
    /// executable.
    slices: Option<slice::Slices>,
    pub inner: InnoInner,
}

/// Where one [`Inno`] reads file data from: the executable itself, or the
/// slices beside it.
#[cfg(feature = "extract")]
pub(crate) enum Source<'a, R> {
    Embedded(read::Embedded<&'a mut R>),
    Slices(&'a mut slice::Slices),
}

#[cfg(feature = "extract")]
impl<R: Read + Seek> Read for Source<'_, R> {
    fn read(&mut self, buffer: &mut [u8]) -> io::Result<usize> {
        match self {
            Self::Embedded(source) => source.read(buffer),
            Self::Slices(source) => source.read(buffer),
        }
    }
}

#[cfg(feature = "extract")]
impl<R: Read + Seek> read::DataSource for Source<'_, R> {
    fn seek_to(&mut self, slice: u32, offset: u64) -> io::Result<()> {
        match self {
            Self::Embedded(source) => source.seek_to(slice, offset),
            Self::Slices(source) => source.seek_to(slice, offset),
        }
    }
}

impl Inno<io::BufReader<fs::File>> {
    /// Reads the installer at `path`, and finds the `.bin` slices beside it
    /// if its data is not in the executable.
    ///
    /// [`new`] plus [`located_at`]; what you want unless the installer is not
    /// a file on disk.
    ///
    /// # Errors
    ///
    /// Returns an error if the file cannot be opened or is not an Inno Setup
    /// installer.
    ///
    /// [`new`]: Self::new
    /// [`located_at`]: Self::located_at
    pub fn open(path: impl AsRef<Path>) -> InnoResult<Self> {
        let path = path.as_ref();
        let reader = io::BufReader::new(fs::File::open(path)?);
        Ok(Self::new(reader)?.located_at(path))
    }
}

impl<R: Read + Seek> Inno<R> {
    pub fn new(mut reader: R) -> InnoResult<Self> {
        let setup_loader =
            SetupLoader::read_from(&mut reader).map_err(|_| InnoError::NotInnoFile)?;

        // Seek to Inno header
        reader.seek(SeekFrom::Start(setup_loader.header_offset().unsigned_abs()))?;

        let mut inno_version = InnoVersion::read(&mut reader)?;

        if inno_version > MAX_SUPPORTED_VERSION {
            return Err(InnoError::UnsupportedVersion(inno_version));
        }

        // Inno Setup sometimes didn't increment the version number between versions with breaking
        // changes. If the version is ambiguous, try reading using successive candidate versions
        // until one succeeds.
        let inner = if let Some(mut versions_to_try) =
            inno_version.ambiguous_candidates().map(Vec::into_iter)
        {
            let position = reader.stream_position()?;

            loop {
                match InnoInner::read_stream(&mut reader, setup_loader, inno_version) {
                    Ok(inno) => break Ok(inno),
                    Err(err) => {
                        if let Some(next) = versions_to_try.next() {
                            inno_version = next;
                            reader.seek(SeekFrom::Start(position))?;
                        } else {
                            break Err(err);
                        }
                    }
                }
            }
        } else {
            InnoInner::read_stream(&mut reader, setup_loader, inno_version)
        }?;

        Ok(Self {
            reader,
            slices: None,
            inner,
        })
    }

    /// Tells this installer where it was read from, so `.bin` slices beside
    /// it can be found. [`open`] does this for you; [`new`] cannot, because a
    /// reader does not say where it came from.
    ///
    /// No effect on an installer that keeps its data in the executable.
    ///
    /// [`new`]: Self::new
    /// [`open`]: Self::open
    pub fn located_at(mut self, path: &Path) -> Self {
        if self.inner.setup_loader.data_offset() == 0 {
            self.slices = Some(slice::Slices::beside_named(
                path,
                self.inner.header.base_filename(),
                self.inner.header.slices_per_disk_checked(),
            ));
        }

        self
    }

    #[inline]
    pub const fn setup_loader(&self) -> &SetupLoader {
        self.inner.setup_loader()
    }

    pub const fn header(&self) -> &Header {
        self.inner.header()
    }

    /// Returns the Inno Setup version.
    #[must_use]
    #[inline]
    pub const fn version(&self) -> InnoVersion {
        self.inner.version()
    }

    /// Returns the encryption header, if any.
    #[must_use]
    pub fn encryption_header(&self) -> Option<&EncryptionHeader> {
        self.inner.encryption_header()
    }

    /// Returns the primary language of the installer, if available.
    #[must_use]
    #[inline]
    pub const fn primary_language(&self) -> Option<&Language> {
        self.inner.primary_language()
    }

    /// Returns the languages as a slice.
    #[must_use]
    #[inline]
    pub const fn languages(&self) -> &[Language] {
        self.inner.languages()
    }

    /// Returns the message entries as a slice.
    #[must_use]
    #[inline]
    pub const fn message_entries(&self) -> &[MessageEntry] {
        self.inner.message_entries()
    }

    pub fn messages(&self) -> impl Iterator<Item = Message<'_, '_>> {
        self.inner.messages()
    }

    /// Returns the permission entries as a slice.
    #[must_use]
    #[inline]
    pub const fn permissions(&self) -> &[Permission] {
        self.inner.permissions()
    }

    /// Returns the type entries as a slice.
    #[must_use]
    #[inline]
    pub const fn type_entries(&self) -> &[Type] {
        self.inner.type_entries()
    }

    /// Returns the component entries as a slice.
    #[must_use]
    #[inline]
    pub const fn components(&self) -> &[Component] {
        self.inner.components()
    }

    /// Returns the task entries as a slice.
    #[must_use]
    #[inline]
    pub const fn tasks(&self) -> &[Task] {
        self.inner.tasks()
    }

    /// Returns the directory entries as a slice.
    #[must_use]
    #[inline]
    pub const fn directories(&self) -> &[Directory] {
        self.inner.directories()
    }

    /// Returns the IS Sig Key entries as a slice.
    #[must_use]
    #[inline]
    pub const fn is_sig_keys(&self) -> &[ISSigKey] {
        self.inner.is_sig_keys()
    }

    /// Returns the file entries as a slice.
    #[must_use]
    #[inline]
    pub const fn file_entries(&self) -> &[File] {
        self.inner.file_entries()
    }

    /// Returns the icon entries as a slice.
    #[must_use]
    #[inline]
    pub const fn icon_entries(&self) -> &[Icon] {
        self.inner.icon_entries()
    }

    /// Returns the ini entries as a slice.
    #[must_use]
    #[inline]
    pub const fn ini_entries(&self) -> &[Ini] {
        self.inner.ini_entries()
    }

    /// Returns the registry entries a slice.
    #[must_use]
    #[inline]
    pub const fn registry_entries(&self) -> &[RegistryEntry] {
        self.inner.registry_entries()
    }

    /// Returns the delete entries as a slice.
    #[must_use]
    #[inline]
    pub const fn delete_entries(&self) -> &[DeleteEntry] {
        self.inner.delete_entries()
    }

    /// Returns the uninstall delete entries as a slice.
    #[must_use]
    #[inline]
    pub const fn uninstall_delete_entries(&self) -> &[DeleteEntry] {
        self.inner.uninstall_delete_entries()
    }

    /// Returns the run entries as a slice.
    #[must_use]
    #[inline]
    pub const fn run_entries(&self) -> &[RunEntry] {
        self.inner.run_entries()
    }

    /// Returns the uninstall run entries as a slice.
    #[must_use]
    #[inline]
    pub const fn uninstall_run_entries(&self) -> &[RunEntry] {
        self.inner.uninstall_run_entries()
    }

    /// Returns a reference to the [`Wizard`].
    #[must_use]
    #[inline]
    pub const fn wizard(&self) -> &Wizard {
        self.inner.wizard()
    }

    /// Returns the file locations entries as a slice.
    #[must_use]
    #[inline]
    pub const fn file_locations(&self) -> &[FileLocation] {
        self.inner.file_locations()
    }

    /// Returns an iterator of files.
    ///
    /// If you do not need every file, use [`filtered_files`].
    ///
    /// [`filtered_files`]: Self::filtered_files
    #[cfg(feature = "extract")]
    pub fn files(&mut self) -> FilesIterator<'_, R> {
        FilesIterator::new(self)
    }

    /// Returns an iterator of files that match the given predicate.
    ///
    /// This is more performant than a [`Filter`] on [`files`] since files that
    /// do not match are not attempted to be read and decompressed.
    ///
    /// [`Filter`]: std::iter::Filter
    /// [`files`]: Self::files
    #[cfg(feature = "extract")]
    pub fn filtered_files<P>(&mut self, predicate: P) -> FilteredFilesIterator<'_, R>
    where
        P: FnMut(&ExtractEntry) -> bool,
    {
        FilteredFilesIterator::new(self, predicate)
    }

    /// Returns an iterator over matching files, reading each one as a stream.
    ///
    /// Unlike [`filtered_files`], which yields each file's bytes as a `Vec`,
    /// this holds nothing larger than a fixed buffer. It is not an
    /// [`Iterator`]: the [`FileReader`] it yields borrows the iterator, so
    /// only one can exist at a time, which no `Iterator` can express.
    ///
    /// The checksum is verified as the bytes are read, so a mismatch is only
    /// reported at the end, once the caller already has them. Use
    /// [`filtered_files`] where that matters.
    ///
    /// Reading one byte still pulls 64 KiB from the chunk, so this is not a
    /// cheap way to peek at the start of a file.
    ///
    /// [`filtered_files`]: Self::filtered_files
    #[cfg(feature = "extract")]
    pub fn streaming_files<P>(&mut self, predicate: P) -> StreamingFiles<'_, R>
    where
        P: FnMut(&ExtractEntry) -> bool,
    {
        StreamingFiles::new(self, predicate)
    }
}
