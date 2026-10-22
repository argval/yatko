# inno

[![CI Status](https://github.com/russellbanks/inno/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/russellbanks/inno/actions/workflows/ci.yml?query=branch:main)
[![Latest version](https://img.shields.io/crates/v/inno.svg)](https://crates.io/crates/inno)
[![Documentation](https://docs.rs/inno/badge.svg)](https://docs.rs/inno)
![License](https://img.shields.io/crates/l/inno.svg)

## Usage

Add this to your `Cargo.toml`:

```toml
[dependencies]
inno = "0.7"
```

## Example

```rust
use inno::{Inno, error::InnoResult};

fn main() -> InnoResult<()> {
    let inno_file = File::new("innosetup-7.1.0-x64.exe").unwrap();

    let inno = Inno::new(file)?;

    println!("{}", inno.header().app_name());

    Ok(())
}
```

## Acknowledgements

* [innoextract](https://github.com/dscharrer/innoextract)
* [Inno Setup](https://jrsoftware.org/isinfo.php)

## License

Licensed under either of:

* Apache License, Version 2.0, ([LICENSE-APACHE](LICENSE-APACHE.md) or https://www.apache.org/licenses/LICENSE-2.0)
* MIT license ([LICENSE-MIT](LICENSE-MIT.md) or https://opensource.org/licenses/MIT)

at your option.

### Contribution

Unless you explicitly state otherwise, any contribution intentionally submitted for inclusion in the work by you, as
defined in the Apache-2.0 license, shall be dual licensed as above, without any additional terms or conditions.
