use zerocopy::{IntoBytes, KnownLayout, TryFromBytes, Unaligned};

pub const CALL: u8 = 0xE8;
pub const JMP: u8 = 0xE9;

#[derive(Copy, Clone, TryFromBytes, IntoBytes, KnownLayout, Unaligned)]
#[repr(u8)]
pub enum OpCode {
    Call = CALL,
    Jmp = JMP,
}

#[derive(Copy, Clone, TryFromBytes, IntoBytes, KnownLayout, Unaligned)]
#[repr(C)]
pub struct Instruction {
    opcode: OpCode,
    pub(crate) address: [u8; 4],
}

impl Instruction {
    /// Creates a new [`Instruction`] from an [`OpCode`] and an address.
    pub const fn new(opcode: OpCode, address: u32) -> Self {
        Self {
            opcode,
            address: address.to_le_bytes(),
        }
    }

    /// Returns the sign extension.
    ///
    /// In Inno Setup >= 5.2.0, for `CALL` and `JMP`, this should be 0x00 or 0xFF.
    pub const fn sign_extension(&self) -> u8 {
        self.address[3]
    }

    /// Returns the relative address.
    ///
    /// In Inno Setup >= 5.2.0, this is stored in the bottom 24 bits of the instruction's address.
    pub const fn address(&self) -> u32 {
        u32::from_le_bytes(self.address)
    }
}
