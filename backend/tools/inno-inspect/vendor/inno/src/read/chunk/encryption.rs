#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Encryption {
    #[default]
    Plaintext,
    Arc4Md5,
    Arc4Sha1,
    XChaCha20,
}

impl Encryption {
    /// Returns `true` if there is no encryption.
    #[must_use]
    #[inline]
    pub const fn is_plaintext(self) -> bool {
        matches!(self, Self::Plaintext)
    }
}
