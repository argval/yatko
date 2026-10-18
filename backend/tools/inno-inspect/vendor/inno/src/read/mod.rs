mod block;
pub mod chunk;
pub mod crc32;
#[cfg(feature = "extract")]
pub mod data_chunk;
mod decoder;
mod ext;
pub mod source;
pub mod stream;

pub use ext::ReadBytesExt;
pub use source::DataSource;
#[cfg(feature = "extract")]
pub use source::Embedded;
