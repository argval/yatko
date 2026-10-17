use crate::entry::{File, FileLocation};

/// A file to extract, pairing the logical file entry with its location metadata.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ExtractEntry {
    index: usize,
    file: File,
    location: FileLocation,
}

impl ExtractEntry {
    /// Creates a new [`ExtractEntry`] from a [`File`] and a
    /// [`FileLocation`].
    #[must_use]
    #[inline]
    pub(crate) const fn new(index: usize, file: File, location: FileLocation) -> Self {
        Self {
            index,
            file,
            location,
        }
    }

    /// Returns this entry's index in the installer's file entries.
    ///
    /// The one key that joins a yielded entry back to anything else derived
    /// from that list. A location index will not do: an installer can store
    /// one payload once and install it under several names, so several entries
    /// name one location.
    #[must_use]
    #[inline]
    pub const fn index(&self) -> usize {
        self.index
    }

    /// Returns the file's location index into the data entry list.
    #[must_use]
    #[inline]
    pub const fn location_index(&self) -> u32 {
        self.file.location()
    }

    /// Returns the [`File`] reference.
    #[must_use]
    #[inline]
    pub const fn file(&self) -> &File {
        &self.file
    }

    /// Returns the [`FileLocation`] reference.
    #[must_use]
    #[inline]
    pub const fn file_location(&self) -> &FileLocation {
        &self.location
    }
}
