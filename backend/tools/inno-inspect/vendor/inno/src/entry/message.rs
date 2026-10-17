use std::io;

use encoding_rs::Encoding;
use zerocopy::LE;

use super::Language;
use crate::{ReadBytesExt, string::PascalString};

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct Message<'message, 'language> {
    entry: &'message MessageEntry,
    language: Option<&'language Language>,
}

impl<'message, 'language> Message<'message, 'language> {
    #[must_use]
    pub fn new(entry: &'message MessageEntry, languages: &'language [Language]) -> Self {
        Self {
            language: entry.language(languages),
            entry,
        }
    }

    /// Returns the name of the message as a string slice.
    #[must_use]
    #[inline]
    pub fn name(&self) -> Option<&str> {
        self.entry.name()
    }

    /// Returns the value of the message as a string slice.
    #[must_use]
    #[inline]
    pub fn value(&self) -> Option<&str> {
        self.entry.value()
    }

    /// Returns the language of the message.
    #[must_use]
    #[inline]
    pub const fn language(&self) -> Option<&'language Language> {
        self.language
    }
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct MessageEntry {
    name: Option<String>,
    value: Option<PascalString>,
    language_index: i32,
}

impl MessageEntry {
    pub fn read<R>(
        mut reader: R,
        languages: &[Language],
        codepage: &'static Encoding,
    ) -> io::Result<Self>
    where
        R: io::Read,
    {
        let mut message = Self {
            name: reader.read_decoded_pascal_string(codepage)?,
            ..Self::default()
        };

        message.value = reader.read_pascal_string()?;

        message.language_index = reader.read_i32::<LE>()?;

        let codepage = if let Ok(index) = usize::try_from(message.language_index)
            && let Some(language) = languages.get(index)
        {
            language.codepage()
        } else {
            codepage
        };

        if let Some(value) = &mut message.value {
            value.decode(codepage);
        }

        Ok(message)
    }

    /// Returns the name of the message as a string slice.
    #[must_use]
    #[inline]
    pub fn name(&self) -> Option<&str> {
        self.name.as_deref()
    }

    /// Returns the value of the message as a string slice.
    #[must_use]
    #[inline]
    pub fn value(&self) -> Option<&str> {
        self.value.as_ref().map(PascalString::as_str)
    }

    /// Returns the message's language index into the language entries.
    #[must_use]
    #[inline]
    pub const fn language_index(&self) -> i32 {
        self.language_index
    }

    /// Returns the language of the message.
    #[must_use]
    pub fn language<'languages>(
        &self,
        languages: &'languages [Language],
    ) -> Option<&'languages Language> {
        languages.get(usize::try_from(self.language_index).ok()?)
    }
}
