//! [`StreamingFiles`] with a `Vec` on the end, so the two paths cannot
//! drift apart. A location a second entry names is read again rather than
//! kept, which costs the decode from its chunk's start and is free on an
//! installer that gives each file its own chunk.

use std::io::{self, Read, Seek};

use crate::{
    Inno,
    entry::checksum::ChecksumMismatchError,
    error::{InnoError, InnoResult},
    iterator::{ExtractEntry, StreamingFiles},
};

/// [`Read`] can only carry an [`io::Error`], but this iterator has always
/// reported a mismatch as [`InnoError::ChecksumMismatch`], which callers match
/// on to tell a corrupt file from a failed read.
fn to_inno_error(error: io::Error) -> InnoError {
    match error.downcast::<ChecksumMismatchError>() {
        Ok(inner) => InnoError::ChecksumMismatch {
            location: "extracted file",
            inner,
        },
        Err(error) => InnoError::Io(error),
    }
}

pub struct FilteredFilesIterator<'reader, R: Read + Seek> {
    files: StreamingFiles<'reader, R>,
}

impl<'reader, R: Read + Seek> FilteredFilesIterator<'reader, R> {
    pub fn new<P>(inno: &'reader mut Inno<R>, predicate: P) -> Self
    where
        P: FnMut(&ExtractEntry) -> bool,
    {
        Self {
            files: StreamingFiles::new(inno, predicate),
        }
    }
}

impl<R: Read + Seek> Iterator for FilteredFilesIterator<'_, R> {
    type Item = InnoResult<(ExtractEntry, Vec<u8>)>;

    fn next(&mut self) -> Option<Self::Item> {
        let (entry, mut reader) = match self.files.next()? {
            Ok(pair) => pair,
            Err(error) => return Some(Err(error)),
        };

        // An exact fit, so `read_to_end` probes for the end rather than
        // doubling. That probe is also the read which checks the checksum.
        let size = usize::try_from(entry.file_location().file().size()).unwrap_or(0);
        let mut data = Vec::with_capacity(size);
        if let Err(error) = reader.read_to_end(&mut data) {
            return Some(Err(to_inno_error(error)));
        }

        Some(Ok((entry, data)))
    }

    fn size_hint(&self) -> (usize, Option<usize>) {
        let remaining = self.files.len();
        (remaining, Some(remaining))
    }
}

impl<R: Read + Seek> ExactSizeIterator for FilteredFilesIterator<'_, R> {}

#[cfg(test)]
mod tests {
    use std::io;

    use crate::{
        entry::checksum::Checksum, error::InnoError, iterator::streaming::mismatch_to_io,
    };

    /// Rebuilding this iterator on the streaming reader must not turn a
    /// corrupt file into an `Io` error.
    #[test]
    fn a_checksum_mismatch_keeps_its_own_variant() {
        let mut hasher = Checksum::new_crc32(crc32fast::hash(b"expected")).hasher();
        hasher.update(b"actual");
        let error = super::to_inno_error(mismatch_to_io(hasher.finish().unwrap_err()));

        assert!(matches!(
            error,
            InnoError::ChecksumMismatch {
                location: "extracted file",
                ..
            }
        ));
        assert!(
            error
                .to_string()
                .starts_with("Inno Setup checksum mismatch reading extracted file.")
        );
    }

    #[test]
    fn any_other_failure_stays_an_io_error() {
        let error = super::to_inno_error(io::Error::from(io::ErrorKind::UnexpectedEof));
        assert!(matches!(error, InnoError::Io(_)));
    }
}
