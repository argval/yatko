use std::fmt;

use zerocopy::{FromBytes, Immutable, KnownLayout, LittleEndian, U32};

/// An Inno Color stored in Little-Endian byte order.
#[derive(Clone, Copy, Default, Eq, PartialEq, FromBytes, Immutable, KnownLayout)]
#[repr(transparent)]
pub struct Color(U32<LittleEndian>);

impl Color {
    /// Creates a new Color from an `rgba` value.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::header::Color;
    ///
    /// let color = Color::new(0xFFC8E10A);
    /// assert_eq!(color.to_string(), "#FFC8E10A");
    /// ```
    #[must_use]
    #[inline]
    pub const fn new(rgba: u32) -> Self {
        Self(U32::new(rgba))
    }

    /// Creates a new Color from a `red`, `green`, `blue` and `alpha`.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::header::Color;
    ///
    /// let color = Color::new_rgba(0xFF, 0xC8, 0xE1, 0xA);
    /// assert_eq!(color.to_string(), "#FFC8E10A");
    /// ```
    #[must_use]
    pub const fn new_rgba(red: u8, green: u8, blue: u8, alpha: u8) -> Self {
        Self(U32::from_bytes([alpha, blue, green, red]))
    }

    /// Returns the `red`, `green`, `blue` and `alpha` values as a tuple.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::header::Color;
    ///
    /// let color = Color::new_rgba(0x12, 0x34, 0x56, 0x78);
    /// assert_eq!(color.to_rgba(), (0x12, 0x34, 0x56, 0x78));
    /// ```
    #[must_use]
    pub const fn to_rgba(self) -> (u8, u8, u8, u8) {
        let [alpha, blue, green, red] = self.0.to_bytes();
        (red, green, blue, alpha)
    }

    /// Returns the `red`, `green`, `blue`, and `alpha` values as an array.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::header::Color;
    ///
    /// let color = Color::new_rgba(0x12, 0x34, 0x56, 0x78);
    /// assert_eq!(color.to_bytes(), [0x12, 0x34, 0x56, 0x78]);
    /// ```
    #[must_use]
    pub const fn to_bytes(self) -> [u8; size_of::<u32>()] {
        let [alpha, blue, green, red] = self.0.to_bytes();
        [red, green, blue, alpha]
    }

    /// Returns the inner `u32`.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::header::Color;
    ///
    /// let color = Color::new_rgba(0x12, 0x34, 0x56, 0x78);
    /// assert_eq!(color.get(), 0x12345678);
    /// ```
    #[must_use]
    #[inline]
    pub const fn get(self) -> u32 {
        self.0.get()
    }

    /// Returns the `red` value.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::header::Color;
    ///
    /// let color = Color::new_rgba(0x12, 0x34, 0x56, 0x78);
    /// assert_eq!(color.red(), 0x12);
    /// ```
    #[must_use]
    pub const fn red(self) -> u8 {
        self.0.to_bytes()[3]
    }

    /// Returns the `green` value.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::header::Color;
    ///
    /// let color = Color::new_rgba(0x12, 0x34, 0x56, 0x78);
    /// assert_eq!(color.green(), 0x34);
    /// ```
    #[must_use]
    pub const fn green(self) -> u8 {
        self.0.to_bytes()[2]
    }

    /// Returns the `blue` value.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::header::Color;
    ///
    /// let color = Color::new_rgba(0x12, 0x34, 0x56, 0x78);
    /// assert_eq!(color.blue(), 0x56);
    /// ```
    #[must_use]
    pub const fn blue(self) -> u8 {
        self.0.to_bytes()[1]
    }

    /// Returns the `alpha` value.
    ///
    /// # Examples
    ///
    /// ```
    /// use inno::header::Color;
    ///
    /// let color = Color::new_rgba(0x12, 0x34, 0x56, 0x78);
    /// assert_eq!(color.alpha(), 0x78);
    /// ```
    #[must_use]
    pub const fn alpha(self) -> u8 {
        self.0.to_bytes()[0]
    }
}

impl fmt::Debug for Color {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        let (red, green, blue, alpha) = self.to_rgba();
        write!(
            f,
            "Color(red={red}, green={green}, blue={blue}, alpha={alpha}, {self})",
        )
    }
}

impl fmt::Display for Color {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "#{:08X}", self.0)
    }
}

impl From<U32<LittleEndian>> for Color {
    fn from(rgba: U32<LittleEndian>) -> Self {
        Self(rgba)
    }
}

impl From<u32> for Color {
    fn from(rgba: u32) -> Self {
        Self::new(rgba)
    }
}

impl From<(u8, u8, u8, u8)> for Color {
    fn from((red, green, blue, alpha): (u8, u8, u8, u8)) -> Self {
        Self::new_rgba(red, green, blue, alpha)
    }
}

impl From<[u8; size_of::<u32>()]> for Color {
    fn from([red, green, blue, alpha]: [u8; size_of::<u32>()]) -> Self {
        Self::new_rgba(red, green, blue, alpha)
    }
}
