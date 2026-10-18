# Inno payload reader

This worker helper lists Inno Setup payload files and reads a selected byte range.
It never runs installer scripts or writes extracted files. It opens only the input
installer. It does not discover adjacent `.bin` files or download external data.

Build with Rust 1.93 or newer:

```sh
cargo build --release --locked --manifest-path backend/tools/inno-inspect/Cargo.toml
cargo test --locked --manifest-path backend/tools/inno-inspect/Cargo.toml
```

The `inno` library is vendored from commit
`6bd3b6d20aef9b655e6eaa2b3c9c6bd0331c902f`, version 0.7.0. A small patch retains
per-file installation conditions that upstream discards. The patch, source
hashes, and notices are in `vendor/inno`. Its parser supports Inno Setup through
7.1.x. The library is licensed under MIT or Apache-2.0.
Source and license files are at
<https://github.com/russellbanks/inno/tree/6bd3b6d20aef9b655e6eaa2b3c9c6bd0331c902f>.
`INNO-LICENSE-MIT` contains the library's MIT license notice.
`Cargo.lock` also pins transitive dependencies.

## Commands

`inno-inspect list PATH` writes one JSON object to stdout:

```json
{
  "format": "inno",
  "version": "7.0.0.3",
  "app_name": "Example",
  "app_version": "1.0",
  "architectures_allowed": ["x64compatible"],
  "architectures_disallowed": [],
  "architectures_64bit": ["x64compatible"],
  "files": [{"index": 0, "path": "bin/example.exe", "destination": "{app}\\bin\\example.exe", "name": "example.exe", "size": 12345, "external": false}]
}
```

`version` is the installer data-format version, which can differ from the release
version. Inno Setup 7.1 uses data-format version 7.0.0.3. Architecture values are
installer constraints, including emulation support. They are not proof of the
payload's native CPU. Inspect the executable header for that proof.

`index` is the original file-entry index. Duplicate paths keep separate indices.
Paths are display data, normalized by removing the initial Inno directory constant
and changing backslashes to slashes. `destination` preserves the original constant
so the worker can separate `{app}` payloads from system helpers. Do not use these
values as output paths. `size` is the
decoded byte length. `external` entries cannot be read by this helper.

Each file also includes nullable `check`, `components`, `tasks`, and `languages`
strings from the installer's file conditions. These strings are never evaluated
by this helper. The worker can recognize fixed CPU checks while treating arbitrary
script conditions as unknown.

`inno-inspect read PATH INDEX MAXBYTES [OFFSET]` writes exactly `MAXBYTES` raw
payload bytes, starting at `OFFSET`, to stdout. The default offset is zero. The
range must fit in the decoded file and `MAXBYTES` must be 1 to 16 MiB. Errors go
to stderr with a nonzero exit status. Discard partial stdout when a command fails.

Range reads use a fixed buffer. A late offset in a solid archive still requires
decompression of earlier bytes. The worker must apply process memory and CPU
limits. A prefix read does not verify the full payload checksum.

