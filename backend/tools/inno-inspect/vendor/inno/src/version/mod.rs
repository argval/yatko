mod variant;
pub mod windows_version;

use std::{cmp::Ordering, fmt, io};

pub use variant::VersionVariant;

use crate::error::{InnoError, InnoResult};

#[derive(Clone, Copy, Debug, Default, Eq)]
pub struct InnoVersion {
    major: u8,
    minor: u8,
    patch: u8,
    revision: u8,
    variant: VersionVariant,
}

impl InnoVersion {
    /// The raw length of the version string in bytes.
    const RAW_LEN: usize = 1 << 6;

    /// Creates a new `InnoVersion` with the specified major, minor, patch, and revision.
    ///
    /// Inno Setup versions 6.3.0 and newer are always Unicode.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::version::{InnoVersion, VersionVariant};
    ///
    /// assert_eq!(InnoVersion::new(6, 2, 2, 0).variant(), VersionVariant::empty());
    ///
    /// // Inno Setup versions 6.3.0 and newer are always Unicode.
    /// assert_eq!(InnoVersion::new(6, 3, 0, 0).variant(), VersionVariant::UNICODE);
    /// ```
    #[must_use]
    #[inline]
    pub const fn new(major: u8, minor: u8, patch: u8, revision: u8) -> Self {
        Self {
            major,
            minor,
            patch,
            revision,
            variant: if major > 6 || (major == 6 && minor >= 3) {
                VersionVariant::UNICODE
            } else {
                VersionVariant::empty()
            },
        }
    }

    /// Creates a new `InnoVersion` with the specified major, minor, patch, revision, and variant
    /// flags.
    ///
    /// Inno Setup versions 6.3.0 and newer are always Unicode.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::version::{InnoVersion, VersionVariant};
    ///
    /// let version = InnoVersion::new_with_variant(1, 3, 21, 0, VersionVariant::ISX);
    /// assert_eq!(version.variant(), VersionVariant::ISX);
    ///
    /// // Inno Setup 6.3.0 and newer is always Unicode
    /// let version = InnoVersion::new_with_variant(6, 3, 0, 0, VersionVariant::ISX);
    /// assert_eq!(version.variant(), VersionVariant::ISX | VersionVariant::UNICODE);
    /// ```
    #[must_use]
    #[inline]
    pub const fn new_with_variant(
        major: u8,
        minor: u8,
        patch: u8,
        revision: u8,
        variant: VersionVariant,
    ) -> Self {
        Self {
            major,
            minor,
            patch,
            revision,
            variant: if major > 6 || (major == 6 && minor >= 3) {
                variant.union(VersionVariant::UNICODE)
            } else {
                variant
            },
        }
    }

    pub fn read<R>(mut reader: R) -> InnoResult<Self>
    where
        R: io::Read,
    {
        let mut raw_version = [0; Self::RAW_LEN];

        reader.read_exact(&mut raw_version)?;

        Self::from_raw_version(&raw_version).ok_or_else(|| {
            InnoError::UnknownVersion(String::from_utf8_lossy(&raw_version).into_owned())
        })
    }

    #[must_use]
    pub fn from_raw_version(mut raw_version: &[u8]) -> Option<Self> {
        const ISX: &[u8; 3] = b"ISX";
        const INNO_SETUP_EXTENSIONS: &[u8; 21] = b"Inno Setup Extensions";

        // Trim trailing null bytes
        if let Some(null_pos) = raw_version.iter().rposition(|&byte| byte != b'\0') {
            raw_version = &raw_version[..=null_pos];
        }

        // Extract the version within the parentheses
        let (version, remaining) = if let Some(start) =
            raw_version.iter().position(|&byte| byte == b'(')
            && let Some(end) = raw_version[start..].iter().position(|&byte| byte == b')')
        {
            (
                &raw_version[start + 1..start + end],
                &raw_version[start + end + 1..],
            )
        } else {
            return None;
        };

        // Split the version string into its components by a `.`
        let mut parts = version
            .split(|&byte| byte == b'.')
            .filter_map(|s| std::str::from_utf8(s).ok()?.parse::<u8>().ok());

        let inno_version = Self::new(
            parts.next()?,
            parts.next()?,
            parts.next()?,
            parts.next().unwrap_or_default(),
        );

        // Inno Setup 6.3.0 and above is always only Unicode
        if inno_version >= 6.3 {
            return Some(inno_version);
        }

        let mut flags = VersionVariant::empty();

        // Check for a Unicode "(u)" flag within parentheses
        if let Some(u_start) = remaining.iter().position(|&byte| byte == b'(')
            && let Some(u_end) = remaining[u_start..].iter().position(|&byte| byte == b')')
            && remaining[u_start + 1..u_start + u_end].eq_ignore_ascii_case(b"u")
        {
            flags |= VersionVariant::UNICODE;
        }

        // Check for "ISX" or "Inno Setup Extensions"
        if remaining.windows(ISX.len()).any(|window| window == ISX)
            || raw_version
                .windows(INNO_SETUP_EXTENSIONS.len())
                .any(|window| window == INNO_SETUP_EXTENSIONS)
        {
            flags |= VersionVariant::ISX;
        }

        Some(Self {
            variant: flags,
            ..inno_version
        })
    }

    /// Returns the major version number.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::version::InnoVersion;
    ///
    /// assert_eq!(InnoVersion::new(6, 4, 0, 1).major(), 6);
    /// ```
    #[must_use]
    #[inline]
    pub const fn major(self) -> u8 {
        self.major
    }

    /// Returns the minor version number.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::version::InnoVersion;
    ///
    /// assert_eq!(InnoVersion::new(6, 4, 0, 1).minor(), 4);
    /// ```
    #[must_use]
    #[inline]
    pub const fn minor(self) -> u8 {
        self.minor
    }

    /// Returns the patch version number.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::version::InnoVersion;
    ///
    /// assert_eq!(InnoVersion::new(6, 4, 0, 1).patch(), 0);
    /// ```
    #[must_use]
    #[inline]
    pub const fn patch(self) -> u8 {
        self.patch
    }

    /// Returns the revision version number.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::version::InnoVersion;
    ///
    /// assert_eq!(InnoVersion::new(6, 4, 0, 1).revision(), 1);
    /// ```
    #[must_use]
    #[inline]
    pub const fn revision(self) -> u8 {
        self.revision
    }

    /// Returns the variant flags of the version.
    ///
    /// # Examples
    ///
    ///
    /// ```
    /// use inno::version::{InnoVersion, VersionVariant};
    ///
    /// assert_eq!(InnoVersion::new(6, 2, 2, 0).variant(), VersionVariant::empty());
    ///
    /// // Inno Setup versions 6.3.0 and newer are always Unicode.
    /// assert_eq!(InnoVersion::new(6, 3, 0, 0).variant(), VersionVariant::UNICODE);
    /// ```
    #[must_use]
    #[inline]
    pub const fn variant(&self) -> VersionVariant {
        self.variant
    }

    /// Returns the version as a tuple of (major, minor, patch, revision).
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::version::InnoVersion;
    ///
    /// assert_eq!(InnoVersion::new(6, 4, 0, 1), (6, 4, 0, 1));
    /// ```
    #[must_use]
    #[inline]
    pub const fn as_tuple(&self) -> (u8, u8, u8, u8) {
        (self.major, self.minor, self.patch, self.revision)
    }

    /// Returns `true` if the version has a Unicode flag.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::version::{InnoVersion, VersionVariant};
    ///
    /// assert!(!InnoVersion::new(6, 2, 0, 0).is_unicode());
    ///
    /// assert!(InnoVersion::new_with_variant(6, 2, 0, 0, VersionVariant::UNICODE).is_unicode());
    ///
    /// assert!(InnoVersion::new(6, 3, 0, 0).is_unicode());
    /// ```
    #[must_use]
    #[inline]
    pub const fn is_unicode(&self) -> bool {
        self.variant.is_unicode()
    }

    /// Returns `true` if the version has an ISX flag.
    #[must_use]
    #[inline]
    pub const fn is_isx(&self) -> bool {
        self.variant.is_isx()
    }

    /// Returns `true` if the version has a 16-bit flag.
    #[must_use]
    #[inline]
    pub const fn is_16_bit(&self) -> bool {
        self.variant.is_16_bit()
    }

    /// Returns `true` if the version is one that was not incremented since a previous Inno Setup
    /// version and therefore may not actually be the true version.
    #[must_use]
    pub fn is_ambiguous(&self) -> bool {
        const AMBIGUOUS_VERSIONS: [InnoVersion; 9] = [
            InnoVersion::new(1, 3, 21, 0), // 1.3.21 or 1.3.24
            InnoVersion::new(2, 0, 1, 0),  // 2.0.1 or 2.0.2
            InnoVersion::new(3, 0, 3, 0),  // 3.0.3 or 3.0.4
            InnoVersion::new(4, 2, 3, 0),  // 4.2.3 or 4.2.4
            InnoVersion::new(5, 3, 10, 0), // 5.3.10 or 5.3.10.1
            InnoVersion::new(5, 4, 2, 0),  // 5.4.2 or 5.4.2.1
            InnoVersion::new(5, 5, 0, 0),  // 5.5.0 or 5.5.0.1
            InnoVersion::new(5, 5, 7, 0),  // 5.5.7 or 5.6.0
            InnoVersion::new(5, 5, 7, 1),  // 5.5.7 or unknown modification
        ];

        AMBIGUOUS_VERSIONS.contains(self)
    }

    /// Returns `true` if the version is a `BlackBox` V2 version.
    #[must_use]
    pub fn is_blackbox(&self) -> bool {
        const BLACKBOX_VERSIONS: [InnoVersion; 3] = [
            InnoVersion::new(5, 3, 10, 0),
            InnoVersion::new(5, 4, 2, 0),
            InnoVersion::new(5, 5, 0, 0),
        ];

        self.is_unicode() && BLACKBOX_VERSIONS.contains(self)
    }

    pub(crate) fn ambiguous_candidates(self) -> Option<Vec<Self>> {
        match self {
            Self {
                major: 1,
                minor: 3,
                patch: 21,
                revision: 0,
                ..
            } => Some(vec![
                Self::new_with_variant(1, 3, 22, 0, self.variant),
                Self::new_with_variant(1, 3, 23, 0, self.variant),
                Self::new_with_variant(1, 3, 24, 0, self.variant),
            ]),
            Self {
                major: 2,
                minor: 0,
                patch: 1,
                revision: 0,
                ..
            } => Some(vec![Self::new_with_variant(2, 0, 2, 0, self.variant)]),
            Self {
                major: 3,
                minor: 0,
                patch: 3,
                revision: 0,
                ..
            } => Some(vec![Self::new_with_variant(3, 0, 4, 0, self.variant)]),
            Self {
                major: 4,
                minor: 2,
                patch: 3,
                revision: 0,
                ..
            } => Some(vec![Self::new_with_variant(4, 2, 4, 0, self.variant)]),
            Self {
                major: 5,
                minor: 3,
                patch: 10,
                revision: 0,
                ..
            } => Some(vec![Self::new_with_variant(5, 3, 10, 1, self.variant)]),
            Self {
                major: 5,
                minor: 4,
                patch: 2,
                revision: 0,
                ..
            } => Some(vec![Self::new_with_variant(5, 4, 2, 1, self.variant)]),
            Self {
                major: 5,
                minor: 5,
                patch: 0,
                revision: 0,
                ..
            } => Some(vec![Self::new_with_variant(5, 5, 0, 1, self.variant)]),
            Self {
                major: 5,
                minor: 5,
                patch: 7,
                revision: 0 | 1,
                ..
            } => Some(vec![
                Self::new_with_variant(5, 5, 8, 0, self.variant),
                Self::new_with_variant(5, 5, 9, 0, self.variant),
                Self::new_with_variant(5, 6, 0, 0, self.variant),
            ]),
            _ => None,
        }
    }
}

impl fmt::Display for InnoVersion {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}.{}", self.major, self.minor)?;

        if self.patch != u8::MAX {
            write!(f, ".{}", self.patch)?;

            if self.revision != u8::MIN && self.revision != u8::MAX {
                write!(f, ".{}", self.revision)?;
            }
        }

        if self.is_16_bit() {
            write!(f, " 16-bit")?;
        }

        if self.is_isx() {
            write!(f, " with ISX")?;
        }

        if self.is_unicode() && *self < (6, 3, 0) {
            write!(f, " (u)")?;
        }

        Ok(())
    }
}

impl PartialEq for InnoVersion {
    fn eq(&self, other: &Self) -> bool {
        self.major == other.major
            && self.minor == other.minor
            && self.patch == other.patch
            && self.revision == other.revision
    }
}

impl PartialOrd for InnoVersion {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for InnoVersion {
    fn cmp(&self, other: &Self) -> Ordering {
        self.major
            .cmp(&other.major)
            .then_with(|| self.minor.cmp(&other.minor))
            .then_with(|| self.patch.cmp(&other.patch))
            .then_with(|| self.revision.cmp(&other.revision))
    }
}

impl PartialEq<(u8, u8, u8, u8)> for InnoVersion {
    fn eq(&self, &(major, minor, patch, revision): &(u8, u8, u8, u8)) -> bool {
        *self == Self::new(major, minor, patch, revision)
    }
}

impl PartialEq<(u8, u8, u8)> for InnoVersion {
    fn eq(&self, &(major, minor, patch): &(u8, u8, u8)) -> bool {
        *self == Self::new(major, minor, patch, 0)
    }
}

impl PartialEq<(u8, u8)> for InnoVersion {
    fn eq(&self, &(major, minor): &(u8, u8)) -> bool {
        *self == Self::new(major, minor, 0, 0)
    }
}

impl PartialEq<u8> for InnoVersion {
    fn eq(&self, &major: &u8) -> bool {
        *self == Self::new(major, 0, 0, 0)
    }
}

impl PartialEq<f32> for InnoVersion {
    #[expect(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
    fn eq(&self, &version: &f32) -> bool {
        *self == Self::new(version as u8, ((version * 10.0) as u8) % 10, 0, 0)
    }
}

impl PartialEq<InnoVersion> for (u8, u8, u8, u8) {
    fn eq(&self, version: &InnoVersion) -> bool {
        *self == version.as_tuple()
    }
}

impl PartialEq<InnoVersion> for (u8, u8, u8) {
    fn eq(&self, version: &InnoVersion) -> bool {
        (self.0, self.1, self.2, 0) == version.as_tuple()
    }
}

impl PartialEq<InnoVersion> for (u8, u8) {
    fn eq(&self, version: &InnoVersion) -> bool {
        (self.0, self.1, 0, 0) == version.as_tuple()
    }
}

impl PartialEq<InnoVersion> for u8 {
    fn eq(&self, version: &InnoVersion) -> bool {
        (*self, 0, 0, 0) == version.as_tuple()
    }
}

impl PartialEq<InnoVersion> for f32 {
    #[expect(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
    fn eq(&self, version: &InnoVersion) -> bool {
        (*self as u8, ((self * 10.0) as u8) % 10, 0, 0) == version.as_tuple()
    }
}

impl PartialOrd<(u8, u8, u8, u8)> for InnoVersion {
    fn partial_cmp(&self, version: &(u8, u8, u8, u8)) -> Option<Ordering> {
        self.as_tuple().partial_cmp(version)
    }
}

impl PartialOrd<(u8, u8, u8)> for InnoVersion {
    fn partial_cmp(&self, &(major, minor, patch): &(u8, u8, u8)) -> Option<Ordering> {
        self.partial_cmp(&Self::new(major, minor, patch, 0))
    }
}

impl PartialOrd<(u8, u8)> for InnoVersion {
    fn partial_cmp(&self, &(major, minor): &(u8, u8)) -> Option<Ordering> {
        self.partial_cmp(&Self::new(major, minor, 0, 0))
    }
}

impl PartialOrd<u8> for InnoVersion {
    fn partial_cmp(&self, &major: &u8) -> Option<Ordering> {
        self.partial_cmp(&Self::new(major, 0, 0, 0))
    }
}

impl PartialOrd<f32> for InnoVersion {
    #[expect(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
    fn partial_cmp(&self, &version: &f32) -> Option<Ordering> {
        self.partial_cmp(&Self::new(
            version as u8,
            ((version * 10.0) as u8) % 10,
            0,
            0,
        ))
    }
}

impl PartialOrd<InnoVersion> for (u8, u8, u8, u8) {
    fn partial_cmp(&self, version: &InnoVersion) -> Option<Ordering> {
        self.partial_cmp(&version.as_tuple())
    }
}

impl PartialOrd<InnoVersion> for (u8, u8, u8) {
    fn partial_cmp(&self, version: &InnoVersion) -> Option<Ordering> {
        (self.0, self.1, self.2, 0).partial_cmp(&version.as_tuple())
    }
}

impl PartialOrd<InnoVersion> for (u8, u8) {
    fn partial_cmp(&self, version: &InnoVersion) -> Option<Ordering> {
        (self.0, self.1, 0, 0).partial_cmp(&version.as_tuple())
    }
}

impl PartialOrd<InnoVersion> for u8 {
    fn partial_cmp(&self, version: &InnoVersion) -> Option<Ordering> {
        (*self, 0, 0, 0).partial_cmp(&version.as_tuple())
    }
}

impl PartialOrd<InnoVersion> for f32 {
    #[expect(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
    fn partial_cmp(&self, version: &InnoVersion) -> Option<Ordering> {
        (*self as u8, ((self * 10.0) as u8) % 10, 0, 0).partial_cmp(&version.as_tuple())
    }
}

#[cfg(test)]
mod tests {
    use std::cmp::Ordering;

    use rstest::rstest;

    use super::{InnoVersion, VersionVariant};

    #[test]
    fn size() {
        assert_eq!(size_of::<InnoVersion>(), 5);
    }

    #[rstest]
    #[case(b"", InnoVersion::new(0, 0, 0, 0))]
    #[case(b"Inno Setup Setup Data (1.3.3)", InnoVersion::new(1, 3, 3, 0))]
    #[case(
        b"Inno Setup Setup Data (1.3.12) with ISX (1.3.12.1)",
        InnoVersion::new_with_variant(1, 3, 12, 0, VersionVariant::ISX)
    )]
    #[case(
        b"Inno Setup Setup Data (3.0.3) with ISX (3.0.0)",
        InnoVersion::new_with_variant(3, 0, 3, 0, VersionVariant::ISX)
    )]
    #[case(
        b"My Inno Setup Extensions Setup Data (3.0.4)",
        InnoVersion::new_with_variant(3, 0, 4, 0, VersionVariant::ISX)
    )]
    #[case(
        b"My Inno Setup Extensions Setup Data (3.0.6.1)",
        InnoVersion::new_with_variant(3, 0, 6, 1, VersionVariant::ISX)
    )]
    #[case(b"Inno Setup Setup Data (5.3.10)", InnoVersion::new(5, 3, 10, 0))]
    #[case(
        b"Inno Setup Setup Data (5.3.10) (u)",
        InnoVersion::new_with_variant(5, 3, 10, 0, VersionVariant::UNICODE)
    )]
    #[case(
        b"Inno Setup Setup Data (5.5.7) (U)",
        InnoVersion::new_with_variant(5, 5, 7, 0, VersionVariant::UNICODE)
    )]
    #[case(b"Inno Setup Setup Data (5.6.0)", InnoVersion::new(5, 6, 0, 0))]
    #[case(
        b"Inno Setup Setup Data (5.6.0) (u)",
        InnoVersion::new_with_variant(5, 6, 0, 0, VersionVariant::UNICODE)
    )]
    #[case(
        b"Inno Setup Setup Data (6.1.0) (u)",
        InnoVersion::new_with_variant(6, 1, 0, 0, VersionVariant::UNICODE)
    )]
    #[case(
        b"Inno Setup Setup Data (6.2.0) (u)",
        InnoVersion::new_with_variant(6, 2, 0, 0, VersionVariant::UNICODE)
    )]
    #[case(
        b"Inno Setup Setup Data (6.3.0)",
        InnoVersion::new_with_variant(6, 3, 0, 0, VersionVariant::UNICODE)
    )]
    #[case(
        b"Inno Setup Setup Data (6.4.0.1)",
        InnoVersion::new_with_variant(6, 4, 0, 1, VersionVariant::UNICODE)
    )]
    fn inno_version_from_bytes(#[case] input: &[u8], #[case] expected_inno_version: InnoVersion) {
        assert_eq!(
            InnoVersion::from_raw_version(input).unwrap_or_default(),
            expected_inno_version
        );
    }

    #[test]
    fn inno_version_equality() {
        let version = InnoVersion::new(1, 2, 3, 4);
        let unicode_version = InnoVersion::new_with_variant(1, 2, 3, 4, VersionVariant::UNICODE);
        let isx_version = InnoVersion::new_with_variant(1, 2, 3, 4, VersionVariant::ISX);

        // Check that version flags aren't included in comparison
        assert_eq!(version, unicode_version);
        assert_eq!(version, isx_version);
        assert_eq!(unicode_version, isx_version);

        // Check that comparison equality returns the same as normal equality
        assert_eq!(version.cmp(&unicode_version), Ordering::Equal);
        assert_eq!(version.cmp(&isx_version), Ordering::Equal);
        assert_eq!(unicode_version.cmp(&isx_version), Ordering::Equal);
    }

    #[test]
    fn inno_version_tuple_equality() {
        // Check equality of `PartialEq<(u8, X, X, X)> for InnoVersion`
        assert_eq!(InnoVersion::new(1, 2, 3, 4), (1, 2, 3, 4));
        assert_eq!(InnoVersion::new(1, 2, 3, 0), (1, 2, 3));
        assert_eq!(InnoVersion::new(1, 2, 0, 0), (1, 2));
        assert_eq!(InnoVersion::new(1, 0, 0, 0), 1);

        // Check inequality of `PartialEq<(u8, X, X, X)> for InnoVersion`
        assert_ne!(InnoVersion::new(1, 2, 3, 4), (4, 3, 2, 1));
        assert_ne!(InnoVersion::new(1, 2, 3, 4), (1, 2, 3));
        assert_ne!(InnoVersion::new(1, 2, 3, 4), (1, 2));
        assert_ne!(InnoVersion::new(1, 2, 3, 4), 1);

        // Check that `PartialCmp<(u8, X, X, X) for InnoVersion` returns the same equality
        assert_eq!(
            InnoVersion::new(1, 2, 3, 4).partial_cmp(&(1, 2, 3, 4)),
            Some(Ordering::Equal)
        );
        assert_eq!(
            InnoVersion::new(1, 2, 3, 0).partial_cmp(&(1, 2, 3)),
            Some(Ordering::Equal)
        );
        assert_eq!(
            InnoVersion::new(1, 2, 0, 0).partial_cmp(&(1, 2)),
            Some(Ordering::Equal)
        );
        assert_eq!(
            InnoVersion::new(1, 0, 0, 0).partial_cmp(&1),
            Some(Ordering::Equal)
        );

        // Check equality of `PartialEq<InnoVersion> for (u8, X, X, X)`
        assert_eq!((1, 2, 3, 4), InnoVersion::new(1, 2, 3, 4));
        assert_eq!((1, 2, 3), InnoVersion::new(1, 2, 3, 0));
        assert_eq!((1, 2), InnoVersion::new(1, 2, 0, 0));
        assert_eq!(1, InnoVersion::new(1, 0, 0, 0));

        // Check inequality of `PartialEq<InnoVersion> for (u8, X, X, X)`
        assert_ne!((1, 2, 3, 4), InnoVersion::new(4, 3, 2, 1));
        assert_ne!((1, 2, 3), InnoVersion::new(1, 2, 3, 4));
        assert_ne!((1, 2), InnoVersion::new(1, 2, 3, 4));
        assert_ne!(1, InnoVersion::new(1, 2, 3, 4));

        // Check that `PartialCmp<InnoVersion> for (u8, X, X, X)` returns the same equality
        assert_eq!(
            (1, 2, 3, 4).partial_cmp(&InnoVersion::new(1, 2, 3, 4)),
            Some(Ordering::Equal)
        );
        assert_eq!(
            (1, 2, 3).partial_cmp(&InnoVersion::new(1, 2, 3, 0)),
            Some(Ordering::Equal)
        );
        assert_eq!(
            (1, 2).partial_cmp(&InnoVersion::new(1, 2, 0, 0)),
            Some(Ordering::Equal)
        );
        assert_eq!(
            1.partial_cmp(&InnoVersion::new(1, 0, 0, 0)),
            Some(Ordering::Equal)
        );
    }

    #[test]
    fn inno_version_float_equality() {
        // Check (in)equality of `PartialEq<f32> for InnoVersion`
        assert_eq!(InnoVersion::new(1, 2, 0, 0), 1.2);
        assert_ne!(InnoVersion::new(1, 2, 3, 4), 1.2);
        assert_ne!(InnoVersion::new(1, 2, 3, 4), 1.234);

        // Check that `PartialCmp<f32> for InnoVersion` returns the same equality
        assert_eq!(
            InnoVersion::new(1, 2, 0, 0).partial_cmp(&1.2),
            Some(Ordering::Equal)
        );
        assert_eq!(
            InnoVersion::new(1, 2, 3, 4).partial_cmp(&1.2),
            Some(Ordering::Greater)
        );

        // Check (in)equality of `PartialEq<InnoVersion> for f32`
        assert_eq!(1.2, InnoVersion::new(1, 2, 0, 0));
        assert_ne!(1.2, InnoVersion::new(1, 2, 3, 4));
        assert_ne!(1.234, InnoVersion::new(1, 2, 3, 4));

        // Check that `PartialCmp<InnoVersion> for f32` returns the same equality
        assert_eq!(
            1.2.partial_cmp(&InnoVersion::new(1, 2, 0, 0)),
            Some(Ordering::Equal)
        );
        assert_eq!(
            1.2.partial_cmp(&InnoVersion::new(1, 2, 3, 4)),
            Some(Ordering::Less)
        );
    }

    #[test]
    fn inno_version_comparison() {
        let version = InnoVersion::new(1, 2, 3, 4);

        assert!(version < InnoVersion::new(1, 2, 3, 5));
        assert!(version > InnoVersion::new(1, 2, 3, 3));

        assert!(version < (1, 2, 3, 5));
        assert!(version > (1, 2, 3, 3));

        assert!(version > (1, 2, 3));
        assert!(version < (1, 2, 4));
    }
}
