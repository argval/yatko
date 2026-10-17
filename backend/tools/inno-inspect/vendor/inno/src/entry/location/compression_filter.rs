use std::{borrow::Cow, io, io::Read};

use flate2::read::ZlibDecoder;
use zerocopy::TryFromBytes;

use super::instruction::Instruction;

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum CompressionFilter {
    #[default]
    NoFilter,
    InstructionFilter4108,
    InstructionFilter5200,
    InstructionFilter5309,
    ZlibFilter,
}

impl CompressionFilter {
    /// The block the instruction filters are defined in terms of. Inno Setup
    /// declines to encode an instruction that would straddle one.
    pub(crate) const BLOCK_SIZE: usize = 64 * 1024;

    /// Returns `true` if this compression filter is [`NoFilter`].
    ///
    /// [`NoFilter`]: Self::NoFilter
    #[must_use]
    #[inline]
    pub const fn is_no_filter(&self) -> bool {
        matches!(self, Self::NoFilter)
    }

    /// Returns `true` if this compression filter is [`ZlibFilter`].
    ///
    /// [`ZlibFilter`]: Self::ZlibFilter
    #[must_use]
    #[inline]
    pub const fn is_zlib(&self) -> bool {
        matches!(self, Self::ZlibFilter)
    }

    /// Where the instruction scan stops within `data`. The file's last
    /// instruction-sized window is left alone; a block's is not, having the
    /// rest of the file behind it.
    fn scan_limit(data: &[u8], is_last: bool) -> usize {
        data.len() + usize::from(!is_last)
    }

    /// Converts relative addresses in x86/x64 CALL and JMP instructions to absolute addresses for older Inno Setup versions.
    ///
    /// This modifies `data` in-place.
    fn decode_4108(data: &mut [u8]) {
        Self::decode_4108_at(data, 0, true);
    }

    /// As [`decode_4108`](Self::decode_4108), but `start` is the offset of
    /// `data[0]` within the whole file and `is_last` says whether the file ends
    /// with `data`. Returns where the scan stopped.
    fn decode_4108_at(data: &mut [u8], start: u64, is_last: bool) -> usize {
        let limit = Self::scan_limit(data, is_last);

        let mut position = 0;
        while position + size_of::<Instruction>() < limit {
            let Ok(instruction) = Instruction::try_mut_from_bytes(
                &mut data[position..position + size_of::<Instruction>()],
            ) else {
                position += 1;
                continue;
            };

            let base_address = (start + (position + size_of::<Instruction>()) as u64) as u32;

            // Change the address to be relative to the beginning of the next instruction
            instruction.address = instruction
                .address()
                .wrapping_sub(base_address)
                .to_le_bytes();

            position += size_of::<Instruction>();
        }

        position
    }

    /// Converts relative addresses in x86/x64 CALL and JMP instructions to
    /// absolute addresses if `ENCODE` IS `true`, or the inverse if `ENCODE` is
    /// false.
    ///
    /// This modifies `data` in-place.
    fn transform_call_instructions<const ENCODE: bool, const FLIP_HIGH_BYTE: bool>(
        data: &mut [u8],
    ) {
        Self::transform_call_instructions_at::<ENCODE, FLIP_HIGH_BYTE>(data, 0, true);
    }

    /// As [`transform_call_instructions`](Self::transform_call_instructions),
    /// but `start` is the offset of `data[0]` within the whole file and
    /// `is_last` says whether the file ends with `data`. Returns where the scan
    /// stopped.
    fn transform_call_instructions_at<const ENCODE: bool, const FLIP_HIGH_BYTE: bool>(
        data: &mut [u8],
        start: u64,
        is_last: bool,
    ) -> usize {
        // https://github.com/jrsoftware/issrc/blob/is-6_7_3/Projects/Src/Compression.Base.pas#L172

        let limit = Self::scan_limit(data, is_last);

        let mut position = 0;
        while position + size_of::<Instruction>() < limit {
            // Does it appear to be a CALL or JMP instruction with a relative 32-bit address?
            let Ok(instruction) = Instruction::try_mut_from_bytes(
                &mut data[position..position + size_of::<Instruction>()],
            ) else {
                position += 1;
                continue;
            };

            // Check that the instruction doesn't span a block boundary
            if (start + position as u64) % Self::BLOCK_SIZE as u64
                > (Self::BLOCK_SIZE - size_of::<Instruction>()) as u64
            {
                position += 1;
                continue;
            }

            // If the address' sign extension is not 0x00 or 0xFF, it's not a CALL or JMP
            if !matches!(instruction.sign_extension(), u8::MIN | u8::MAX) {
                position += size_of::<Instruction>();
                continue;
            }

            // Get the base and relative address as 24-bit integers
            let base_address =
                (start + (position + size_of::<Instruction>()) as u64) as u32 & 0x00FF_FFFF;
            let mut relative_address = instruction.address() & 0x00FF_FFFF;

            if !ENCODE {
                // Change the address to be relative to the beginning of the next instruction
                relative_address = relative_address.wrapping_sub(base_address);
            }

            // For a slightly higher compression ratio, Inno Setup >= 5.3.0.9
            // wants the resulting high byte to be 0x00 for both forward and
            // backward jumps. The high byte of the original relative address is
            // likely to be the sign extension of bit 23, so if bit 23 is set,
            // toggle all bits in the high byte.
            if FLIP_HIGH_BYTE && (relative_address & (1 << 23)) != 0 {
                instruction.address[3] = !instruction.address[3];
            }

            if ENCODE {
                // Change the address to be relative to the beginning of the buffer
                relative_address = relative_address.wrapping_add(base_address);
            }

            instruction.address[..3].copy_from_slice(&relative_address.to_le_bytes()[..3]);

            position += size_of::<Instruction>();
        }

        position
    }

    /// Apply the inverse compression filter to extracted file data.
    ///
    /// Inno Setup applies instruction filters before compression to improve the
    /// compression ratio of executables. This function reverses those transforms.
    ///
    /// * If the compression filter is [`InstructionFilter4108`],
    ///   [`InstructionFilter5200`], [`InstructionFilter5309`], `data` is
    ///   modified in-place and the returned [`Cow`] is a reference to the data.
    /// * If the compression filter is [`ZlibFilter`], `data` is not modified
    ///   and the returned [`Cow`] is the owned decompressed data.
    /// * If the compression filter is [`NoFilter`], `data` is not modified
    ///   and the returned [`Cow`] is a reference to the data.
    ///
    /// [`InstructionFilter4108`]: Self::InstructionFilter4108
    /// [`InstructionFilter5200`]: Self::InstructionFilter5200
    /// [`InstructionFilter5309`]: Self::InstructionFilter5309
    /// [`ZlibFilter`]: Self::ZlibFilter
    /// [`NoFilter`]: Self::NoFilter
    pub fn decode(self, data: &mut [u8]) -> io::Result<Cow<'_, [u8]>> {
        match self {
            Self::NoFilter => {}
            Self::InstructionFilter4108 => Self::decode_4108(data),
            Self::InstructionFilter5200 => Self::transform_call_instructions::<false, false>(data),
            Self::InstructionFilter5309 => Self::transform_call_instructions::<false, true>(data),
            Self::ZlibFilter => {
                // Create a buffer that is at least the size of the compressed data
                let mut decompressed = Vec::with_capacity(data.len());
                ZlibDecoder::new(&*data).read_to_end(&mut decompressed)?;
                return Ok(Cow::Owned(decompressed));
            }
        }

        Ok(Cow::Borrowed(data))
    }

    /// Applies the inverse filter to one block of a file, where `position` is
    /// the offset of `block[0]` within the whole file and `is_last` says
    /// whether the file ends with `block`.
    ///
    /// Returns how much of `block` was decoded, holding back any trailing bytes
    /// an instruction could run past the end of. Moving those to the front of
    /// the next block keeps the scan one continuous walk, and so agrees with
    /// [`decode`](Self::decode) byte for byte.
    ///
    /// [`ZlibFilter`] is a nested stream rather than a transform, so a
    /// streaming caller wraps the source in a decoder instead.
    ///
    /// [`ZlibFilter`]: Self::ZlibFilter
    pub(crate) fn decode_block(self, block: &mut [u8], position: u64, is_last: bool) -> usize {
        let scanned = match self {
            Self::InstructionFilter4108 => Self::decode_4108_at(block, position, is_last),
            Self::InstructionFilter5200 => {
                Self::transform_call_instructions_at::<false, false>(block, position, is_last)
            }
            Self::InstructionFilter5309 => {
                Self::transform_call_instructions_at::<false, true>(block, position, is_last)
            }
            Self::NoFilter | Self::ZlibFilter => block.len(),
        };

        if is_last { block.len() } else { scanned }
    }
}

#[cfg(test)]
mod tests {
    use zerocopy::transmute;

    use super::{super::instruction::OpCode, CompressionFilter, Instruction};

    #[test]
    fn no_filter_is_noop() {
        let instruction = Instruction::new(OpCode::Call, 0x04030201);
        let mut data: [u8; size_of::<Instruction>()] = transmute!(instruction);
        let original = data;

        CompressionFilter::NoFilter.decode(&mut data).unwrap();

        assert_eq!(data, original);
    }

    #[test]
    fn filter_4108_no_call() {
        /// No-operation opcode
        const NOP: u8 = 0x90;

        let mut data = [NOP; 4];
        let original = data;

        CompressionFilter::InstructionFilter4108
            .decode(&mut data)
            .unwrap();

        assert_eq!(data, original);
    }

    #[test]
    fn filter_5200_no_transform_non_sign_extended() {
        // High byte is 0x42, not 0x00 or 0xFF - should not transform
        let instruction = Instruction::new(OpCode::Call, 0x42302010);
        let mut data: [u8; size_of::<Instruction>()] = transmute!(instruction);
        let original = data;

        CompressionFilter::InstructionFilter5200
            .decode(&mut data)
            .unwrap();

        assert_eq!(data, original);
    }
}

#[cfg(test)]
mod block_tests {
    use super::{CompressionFilter, Instruction};

    const BLOCK: usize = CompressionFilter::BLOCK_SIZE;

    /// Drives `decode_block` the way the streaming reader does, carrying what
    /// a block holds back over to the front of the next one.
    fn decode_in_blocks(filter: CompressionFilter, data: &[u8]) -> Vec<u8> {
        let mut decoded = Vec::with_capacity(data.len());
        let mut block = vec![0u8; BLOCK];
        let mut remaining = data;
        let mut carry = 0;
        let mut position = 0;

        while !remaining.is_empty() {
            let count = remaining.len().min(BLOCK - carry);
            block[carry..carry + count].copy_from_slice(&remaining[..count]);
            remaining = &remaining[count..];

            let total = carry + count;
            let stop = filter.decode_block(&mut block[..total], position, remaining.is_empty());

            decoded.extend_from_slice(&block[..stop]);
            block.copy_within(stop..total, 0);
            carry = total - stop;
            position += stop as u64;
        }

        decoded
    }

    /// Feeding a location through `decode_block` in blocks must give exactly
    /// what `decode` gives for the whole thing, or a streaming read would
    /// produce different bytes from a buffered one.
    #[test]
    fn block_wise_decoding_matches_whole_buffer_decoding() {
        // A CALL every 16 bytes, so the filter finds something in every block.
        let mut data = vec![0u8; BLOCK * 2 + 4096];
        for (index, chunk) in data.chunks_mut(16).enumerate() {
            if chunk.len() == 16 {
                chunk[0] = 0xE8;
                chunk[1..5].copy_from_slice(&(index as u32).to_le_bytes());
            }
        }

        for filter in [
            CompressionFilter::InstructionFilter4108,
            CompressionFilter::InstructionFilter5200,
            CompressionFilter::InstructionFilter5309,
        ] {
            let mut whole = data.clone();
            let expected = filter.decode(&mut whole).expect("whole").into_owned();

            assert_eq!(decode_in_blocks(filter, &data), expected, "{filter:?}");
        }
    }

    /// A non-final block's last window has the rest of the file behind it, so
    /// unlike the whole-buffer form it must still be decoded.
    #[test]
    fn an_instruction_at_the_end_of_a_block_is_decoded() {
        // Zeroes are not a valid opcode, so the scan steps a byte at a time and
        // is guaranteed to arrive at the one instruction here.
        let mut data = vec![0u8; BLOCK * 2];
        data[BLOCK - size_of::<Instruction>()] = 0xE8;

        for filter in [
            CompressionFilter::InstructionFilter4108,
            CompressionFilter::InstructionFilter5200,
            CompressionFilter::InstructionFilter5309,
        ] {
            let mut whole = data.clone();
            let expected = filter.decode(&mut whole).expect("whole").into_owned();
            assert_ne!(expected, data, "{filter:?} left the instruction alone");

            assert_eq!(decode_in_blocks(filter, &data), expected, "{filter:?}");
        }
    }

    /// Only 4108 encodes an instruction that straddles a block, and only the
    /// carry decodes it: a scan restarting each block steps over it.
    #[test]
    fn an_instruction_straddling_a_block_boundary_is_decoded() {
        let mut data = vec![0u8; BLOCK * 2];
        data[BLOCK - 2] = 0xE8;

        let filter = CompressionFilter::InstructionFilter4108;
        let mut whole = data.clone();
        let expected = filter.decode(&mut whole).expect("whole").into_owned();
        assert_ne!(expected, data, "the straddling instruction was left alone");

        assert_eq!(decode_in_blocks(filter, &data), expected);
    }

    /// 5200 and 5309 refuse a straddling instruction by a rule on absolute file
    /// offsets. The carry moves where a block begins, so check it still does.
    #[test]
    fn the_5200_boundary_rule_skips_a_straddling_instruction() {
        let mut data = vec![0u8; BLOCK * 2];
        data[BLOCK - 2] = 0xE8;

        for filter in [
            CompressionFilter::InstructionFilter5200,
            CompressionFilter::InstructionFilter5309,
        ] {
            let mut whole = data.clone();
            let expected = filter.decode(&mut whole).expect("whole").into_owned();
            assert_eq!(expected, data, "{filter:?} decoded across the boundary");

            assert_eq!(decode_in_blocks(filter, &data), expected, "{filter:?}");
        }
    }

    #[test]
    fn no_filter_leaves_a_block_alone() {
        let mut block = [1u8, 2, 3, 4, 5];
        assert_eq!(
            CompressionFilter::NoFilter.decode_block(&mut block, 0, true),
            5
        );
        assert_eq!(block, [1, 2, 3, 4, 5]);
    }
}
