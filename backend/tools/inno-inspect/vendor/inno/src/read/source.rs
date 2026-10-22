use std::io::{self, Read, Seek, SeekFrom};

/// Where an installer's file data is read from: the setup executable itself,
/// after the header, or the numbered `.bin` slices beside it. A file entry's
/// offsets mean the same in both cases, so which file to look in is the only
/// difference.
pub trait DataSource: Read {
    /// Positions the source `offset` bytes into `slice`.
    ///
    /// # Errors
    ///
    /// Returns an error if the slice cannot be reached, which for an
    /// installer whose data is embedded means any slice but the first.
    fn seek_to(&mut self, slice: u32, offset: u64) -> io::Result<()>;
}

/// Data kept inside the setup executable, after its header.
pub struct Embedded<R> {
    reader: R,
    data_offset: u64,
}

impl<R: Read + Seek> Embedded<R> {
    pub const fn new(reader: R, data_offset: u64) -> Self {
        Self {
            reader,
            data_offset,
        }
    }
}

impl<R: Read> Read for Embedded<R> {
    fn read(&mut self, buffer: &mut [u8]) -> io::Result<usize> {
        self.reader.read(buffer)
    }
}

impl<R: Read + Seek> DataSource for Embedded<R> {
    fn seek_to(&mut self, slice: u32, offset: u64) -> io::Result<()> {
        if slice > 0 {
            // Reading on regardless gives whatever sits at that offset in
            // the executable, which reads as a corrupt archive rather than
            // as a slice that was never opened.
            return Err(io::Error::new(
                io::ErrorKind::NotFound,
                format!(
                    "this installer keeps its data in slices beside it, and slice {slice} was not \
                     supplied"
                ),
            ));
        }

        self.reader
            .seek(SeekFrom::Start(self.data_offset + offset))?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::io::{Cursor, Read};

    use super::{DataSource, Embedded};

    #[test]
    fn an_offset_is_counted_from_the_data_offset() {
        let mut source = Embedded::new(Cursor::new(b"skip meDATA".to_vec()), 7);
        source.seek_to(0, 0).unwrap();

        let mut out = String::new();
        source.read_to_string(&mut out).unwrap();
        assert_eq!(out, "DATA");
    }

    #[test]
    fn asking_an_embedded_installer_for_a_later_slice_says_what_is_missing() {
        // Seeking anyway lands somewhere arbitrary, and the failure surfaces
        // much later as a chunk that will not decompress.
        let mut source = Embedded::new(Cursor::new(vec![0; 64]), 0);
        let error = source.seek_to(1, 0).unwrap_err();
        assert!(error.to_string().contains("slice 1"), "{error}");
    }
}
