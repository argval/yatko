use std::fmt;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct YesNoStr<'a>(&'a str);

impl<'a> YesNoStr<'a> {
    /// Creates a new `YesNoStr` from a string slice.
    #[must_use]
    #[inline]
    pub const fn new(yes_no: &'a str) -> Self {
        Self(yes_no)
    }

    /// Returns `true` if the string matches `yes`, `true`, or `1` (case-insensitive).
    #[must_use]
    pub fn as_bool(&self) -> bool {
        self.as_str().eq_ignore_ascii_case("yes")
            || self.as_str().eq_ignore_ascii_case("true")
            || self.as_str() == "1"
    }

    /// Returns the inner Yes/No as a string slice.
    #[must_use]
    #[inline]
    pub const fn as_str(&self) -> &'a str {
        self.0
    }
}

impl AsRef<str> for YesNoStr<'_> {
    #[inline]
    fn as_ref(&self) -> &str {
        self.as_str()
    }
}

impl fmt::Display for YesNoStr<'_> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}

impl From<YesNoStr<'_>> for bool {
    #[inline]
    fn from(yes_no: YesNoStr<'_>) -> Self {
        yes_no.as_bool()
    }
}

impl<'a> From<&'a str> for YesNoStr<'a> {
    #[inline]
    fn from(yes_no: &'a str) -> Self {
        Self(yes_no)
    }
}
