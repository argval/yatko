use std::io::{Read, Seek};

use super::ExtractEntry;
use crate::{Inno, error::InnoResult, iterator::FilteredFilesIterator};

/// An iterator over the files of an Inno Setup installer.
pub struct FilesIterator<'reader, R: Read + Seek>(FilteredFilesIterator<'reader, R>);

impl<'reader, R: Read + Seek> FilesIterator<'reader, R> {
    /// Creates a new [`FilesIterator`] from a mutable reference to [`Inno`].
    pub fn new(inno: &'reader mut Inno<R>) -> Self {
        Self(FilteredFilesIterator::new(inno, |_| true))
    }
}

impl<R: Read + Seek> Iterator for FilesIterator<'_, R> {
    type Item = InnoResult<(ExtractEntry, Vec<u8>)>;

    fn next(&mut self) -> Option<Self::Item> {
        self.0.next()
    }

    fn size_hint(&self) -> (usize, Option<usize>) {
        self.0.size_hint()
    }
}

impl<R: Read + Seek> ExactSizeIterator for FilesIterator<'_, R> {}
