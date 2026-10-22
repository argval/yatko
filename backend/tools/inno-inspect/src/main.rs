use std::{
    env,
    error::Error,
    ffi::OsString,
    fs::File,
    io::{self, Read, Seek, Write},
    path::PathBuf,
    process::ExitCode,
};

use inno::{Inno, header::Architecture};
use serde::Serialize;

const MAX_READ: u64 = 16 * 1024 * 1024;
const MAX_ENTRIES: usize = 100_000;
const USAGE: &str = "usage: inno-inspect list PATH | read PATH INDEX MAXBYTES [OFFSET]";
type Result<T> = std::result::Result<T, Box<dyn Error>>;

#[derive(Debug, PartialEq)]
enum Command {
    List(PathBuf),
    Read {
        path: PathBuf,
        index: usize,
        maxbytes: u64,
        offset: u64,
    },
}

#[derive(Serialize)]
struct Listing<'a> {
    format: &'static str,
    version: String,
    app_name: Option<&'a str>,
    app_version: Option<&'a str>,
    architectures_allowed: Vec<String>,
    architectures_disallowed: Vec<String>,
    architectures_64bit: Vec<String>,
    files: Vec<Entry>,
}

#[derive(Serialize)]
struct Entry {
    index: usize,
    path: String,
    destination: String,
    name: String,
    size: u64,
    external: bool,
    check: Option<String>,
    components: Option<String>,
    tasks: Option<String>,
    languages: Option<String>,
}

/// Parse the fixed command format and reject unbounded output requests.
fn parse(args: Vec<OsString>) -> Result<Command> {
    match args.as_slice() {
        [command, path] if command == "list" => Ok(Command::List(path.into())),
        [command, path, index, maxbytes, tail @ ..] if command == "read" && tail.len() <= 1 => {
            let index = index.to_str().ok_or("invalid entry index")?.parse()?;
            let maxbytes = maxbytes.to_str().ok_or("invalid byte count")?.parse()?;
            if maxbytes == 0 || maxbytes > MAX_READ {
                return Err("MAXBYTES must be between 1 and 16777216".into());
            }
            let offset = match tail.first() {
                Some(value) => value.to_str().ok_or("invalid offset")?.parse()?,
                None => 0,
            };
            Ok(Command::Read {
                path: path.into(),
                index,
                maxbytes,
                offset,
            })
        }
        _ => Err(USAGE.into()),
    }
}

/// Keep Inno's architecture flags as metadata, separate from payload CPU proof.
fn architecture_names(value: Architecture) -> Vec<String> {
    value
        .iter_names()
        .map(|(name, _)| name.to_ascii_lowercase().replace('_', ""))
        .collect()
}

/// List display paths and stable entry indices without opening payload files.
fn list<R: Read + Seek>(inno: &Inno<R>, output: impl Write) -> Result<()> {
    if inno.file_entries().len() > MAX_ENTRIES {
        return Err("installer exceeds the 100000 entry limit".into());
    }
    let embedded = inno.inner.setup_loader.data_offset() > 0;
    let files = inno
        .file_entries()
        .iter()
        .enumerate()
        .map(|(index, file)| {
            let path = file.normalized_destination().unwrap_or_default();
            let name = path.rsplit('/').next().unwrap_or_default().to_owned();
            let location = inno.file_locations().get(file.location() as usize);
            Entry {
                index,
                path,
                destination: file.destination().unwrap_or_default().to_owned(),
                name,
                size: location.map_or(file.external_size(), |value| value.uncompressed_size()),
                external: !embedded || location.is_none(),
                check: file.condition().check().map(str::to_owned),
                components: file.condition().as_str().map(str::to_owned),
                tasks: file.condition().tasks().map(str::to_owned),
                languages: file.condition().languages().map(str::to_owned),
            }
        })
        .collect();
    let header = inno.header();
    let listing = Listing {
        format: "inno",
        version: inno.version().to_string(),
        app_name: header.app_name(),
        app_version: header.app_version(),
        architectures_allowed: architecture_names(header.architectures_allowed()),
        architectures_disallowed: architecture_names(header.architectures_disallowed()),
        architectures_64bit: architecture_names(header.architectures_install_in_64_bit_mode()),
        files,
    };
    serde_json::to_writer(output, &listing)?;
    Ok(())
}

/// Reject ranges outside the decoded file before any payload reaches stdout.
fn validate_range(size: u64, maxbytes: u64, offset: u64) -> Result<()> {
    if maxbytes == 0 || maxbytes > MAX_READ {
        return Err("MAXBYTES must be between 1 and 16777216".into());
    }
    if offset.checked_add(maxbytes).is_none_or(|end| end > size) {
        return Err("requested range exceeds the decoded file size".into());
    }
    Ok(())
}

/// Copy one bounded range with a fixed buffer and no archive-controlled writes.
fn copy_range(
    mut reader: impl Read,
    mut output: impl Write,
    maxbytes: u64,
    offset: u64,
) -> Result<()> {
    if io::copy(&mut reader.by_ref().take(offset), &mut io::sink())? != offset {
        return Err("payload ended before the requested offset".into());
    }
    if io::copy(&mut reader.take(maxbytes), &mut output)? != maxbytes {
        return Err("payload ended before the requested range".into());
    }
    Ok(())
}

/// Read only the selected embedded entry, never discover adjacent data slices.
fn read<R: Read + Seek>(
    inno: &mut Inno<R>,
    index: usize,
    maxbytes: u64,
    offset: u64,
    output: impl Write,
) -> Result<()> {
    if inno.inner.setup_loader.data_offset() <= 0 {
        return Err("external data slices are unavailable".into());
    }
    let file = inno
        .file_entries()
        .get(index)
        .ok_or("entry index out of range")?;
    let location = inno
        .file_locations()
        .get(file.location() as usize)
        .ok_or("entry has no embedded payload")?;
    validate_range(location.uncompressed_size(), maxbytes, offset)?;
    let mut files = inno.streaming_files(|entry| entry.index() == index);
    let (_, reader) = files.next().ok_or("entry has no embedded payload")??;
    copy_range(reader, output, maxbytes, offset)
}

/// Open the caller's installer and dispatch read-only inspection commands.
fn run() -> Result<()> {
    let command = parse(env::args_os().skip(1).collect())?;
    let path = match &command {
        Command::List(path) | Command::Read { path, .. } => path,
    };
    // Inno::new does not resolve external .bin paths, unlike Inno::open.
    let mut inno = Inno::new(File::open(path)?)?;
    let stdout = io::stdout();
    match command {
        Command::List(_) => list(&inno, stdout.lock()),
        Command::Read {
            index,
            maxbytes,
            offset,
            ..
        } => read(&mut inno, index, maxbytes, offset, stdout.lock()),
    }
}

fn main() -> ExitCode {
    match run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            eprintln!("inno-inspect: {error}");
            ExitCode::FAILURE
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_large_overflowed_and_out_of_file_ranges() {
        assert!(validate_range(u64::MAX, MAX_READ + 1, 0).is_err());
        assert!(validate_range(u64::MAX, 2, u64::MAX).is_err());
        assert!(validate_range(10, 4, 7).is_err());
        assert!(validate_range(10, 3, 7).is_ok());
    }

    #[test]
    fn range_copy_limits_output_and_rejects_truncation() {
        let mut output = Vec::new();
        copy_range(io::Cursor::new(b"0123456789"), &mut output, 3, 5).unwrap();
        assert_eq!(output, b"567");
        assert!(copy_range(io::Cursor::new(b"012"), io::sink(), 1, 4).is_err());
        assert!(copy_range(io::Cursor::new(b"012"), io::sink(), 4, 0).is_err());
    }

    #[test]
    fn read_arguments_require_an_index_and_bounded_length() {
        let args = |values: &[&str]| values.iter().map(OsString::from).collect();
        assert_eq!(
            parse(args(&["read", "setup.exe", "3", "16", "8"])).unwrap(),
            Command::Read {
                path: "setup.exe".into(),
                index: 3,
                maxbytes: 16,
                offset: 8,
            }
        );
        assert!(parse(args(&["read", "setup.exe", "-1", "16"])).is_err());
        assert!(parse(args(&["read", "setup.exe", "0", "0"])).is_err());
        assert!(parse(args(&["read", "setup.exe", "0", "16777217"])).is_err());
        assert!(parse(args(&["read", "setup.exe", "0", "16", "0", "extra"])).is_err());
    }
}
