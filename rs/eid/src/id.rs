//! The 16-byte layout, the alphabet, the canonical text and its strict parse (spec/id.md).

use std::fmt;

use crate::Refusal;

/// The 32 symbols in value order; ASCII order equals value order.
pub const ALPHABET: &[u8; 32] = b"234567abcdefghijklmnopqrstuvwxyz";
/// The length of the canonical text in bytes.
pub const TEXT_LEN: usize = 28;
/// The largest time field: 2^48 - 1 milliseconds.
pub const TIME_MAX: u64 = (1 << 48) - 1;
/// The largest writer number: 2^30 - 1.
pub const WRITER_MAX: u32 = (1 << 30) - 1;
/// Ids per writer per millisecond before a burst borrows the next millisecond.
pub const COUNT_LIMIT: u32 = 1 << 18;

const NOT_IN_ALPHABET: u8 = 0xff;

const fn value_table() -> [u8; 256] {
    let mut table = [NOT_IN_ALPHABET; 256];
    let mut i = 0;
    while i < 32 {
        table[ALPHABET[i] as usize] = i as u8;
        i += 1;
    }
    table
}

static VALUE: [u8; 256] = value_table();

/// A 15-bit code of three alphabet characters: a system code or a kind.
#[derive(Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct Code(u16);

/// The class of a system code, read from its first character.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Class {
    /// A letter: a row in the system registry.
    Registered,
    /// A digit `3`–`7`: drawn at random by a producer with no lease.
    Detached,
    /// The digit `2`: reserved for a later format and refused.
    Reserved,
}

impl Code {
    /// Parses three alphabet characters. Uppercase and any other byte are refused.
    pub fn parse(s: &str) -> Option<Code> {
        let b = s.as_bytes();
        if b.len() != 3 {
            return None;
        }
        let mut v: u16 = 0;
        for &c in b {
            let x = VALUE[c as usize];
            if x == NOT_IN_ALPHABET {
                return None;
            }
            v = (v << 5) | x as u16;
        }
        Some(Code(v))
    }

    /// A code from its 15-bit value.
    pub const fn from_bits(v: u16) -> Option<Code> {
        if v < 1 << 15 { Some(Code(v)) } else { None }
    }

    /// The 15-bit value.
    pub const fn bits(self) -> u16 {
        self.0
    }

    /// The class of this code read as a system code.
    pub const fn class(self) -> Class {
        match self.0 >> 10 {
            0 => Class::Reserved,
            1..=5 => Class::Detached,
            _ => Class::Registered,
        }
    }

    /// Whether this code is a valid kind: three letters.
    pub const fn is_kind(self) -> bool {
        (self.0 >> 10) & 31 >= 6 && (self.0 >> 5) & 31 >= 6 && self.0 & 31 >= 6
    }

    /// The three characters.
    pub fn text(self) -> [u8; 3] {
        [
            ALPHABET[((self.0 >> 10) & 31) as usize],
            ALPHABET[((self.0 >> 5) & 31) as usize],
            ALPHABET[(self.0 & 31) as usize],
        ]
    }
}

impl fmt::Display for Code {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let t = self.text();
        f.write_str(std::str::from_utf8(&t).unwrap_or("???"))
    }
}

impl fmt::Debug for Code {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::Display::fmt(self, f)
    }
}

/// A code from three alphabet characters at compile time; panics during compilation on any other byte.
pub const fn code(s: &[u8; 3]) -> Code {
    const TABLE: [u8; 256] = value_table();
    let (a, b, c) = (TABLE[s[0] as usize], TABLE[s[1] as usize], TABLE[s[2] as usize]);
    assert!(a != NOT_IN_ALPHABET && b != NOT_IN_ALPHABET && c != NOT_IN_ALPHABET);
    Code(((a as u16) << 10) | ((b as u16) << 5) | c as u16)
}

/// The kind that names a system itself, in every system.
pub const SYS: Code = code(b"sys");

/// An Ecd ID. The ordering of the inner `u128` equals the byte order of the 16 bytes.
#[derive(Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct Id(u128);

impl Id {
    /// Builds an id from its fields, checking each against the layout.
    pub fn from_parts(system: Code, kind: Code, time: u64, writer: u32, count: u32) -> Result<Id, Refusal> {
        if system.class() == Class::Reserved {
            return Err(Refusal::SystemReserved);
        }
        if !kind.is_kind() {
            return Err(Refusal::Kind);
        }
        if time > TIME_MAX || writer > WRITER_MAX || count >= COUNT_LIMIT {
            return Err(Refusal::Range);
        }
        Ok(Id::pack(system, kind, time, writer, count))
    }

    pub(crate) fn pack(system: Code, kind: Code, time: u64, writer: u32, count: u32) -> Id {
        Id(((system.0 as u128) << 113)
            | ((kind.0 as u128) << 98)
            | ((time as u128) << 48)
            | ((writer as u128) << 18)
            | count as u128)
    }

    /// The system's own id: its code, kind `sys` and a zero local part (spec/mxdb.md).
    pub fn system_id(system: Code) -> Result<Id, Refusal> {
        Id::from_parts(system, SYS, 0, 0, 0)
    }

    fn check(n: u128) -> Result<Id, Refusal> {
        let id = Id(n);
        if (n >> 96) & 3 != 0 {
            return Err(Refusal::Reserved);
        }
        if id.system().class() == Class::Reserved {
            return Err(Refusal::SystemReserved);
        }
        if !id.kind().is_kind() {
            return Err(Refusal::Kind);
        }
        Ok(id)
    }

    /// Binary ingress: exactly 16 bytes, then steps 5–7 of the strict parse.
    pub fn from_bytes(b: &[u8]) -> Result<Id, Refusal> {
        let arr: [u8; 16] = b.try_into().map_err(|_| Refusal::Length)?;
        Id::check(u128::from_be_bytes(arr))
    }

    /// The 16 bytes, big-endian.
    pub fn to_bytes(self) -> [u8; 16] {
        self.0.to_be_bytes()
    }

    /// The strict parse of the canonical text (spec/id.md#strict-parse).
    pub fn parse(t: &[u8]) -> Result<Id, Refusal> {
        if t.len() != TEXT_LEN {
            return Err(Refusal::Length);
        }
        if t[3] != b'_' || t[7] != b'_' {
            return Err(Refusal::Separator);
        }
        let mut n: u128 = 0;
        let mut last: u8 = 0;
        let mut i = 0;
        for (pos, &c) in t.iter().enumerate() {
            if pos == 3 || pos == 7 {
                continue;
            }
            let v = VALUE[c as usize];
            if v == NOT_IN_ALPHABET {
                return Err(Refusal::Alphabet);
            }
            if i < 25 {
                n = (n << 5) | v as u128;
            } else {
                last = v;
            }
            i += 1;
        }
        if last & 3 != 0 {
            return Err(Refusal::Pad);
        }
        Id::check((n << 3) | (last >> 2) as u128)
    }

    /// The canonical text as 28 bytes.
    pub fn encode(self) -> [u8; TEXT_LEN] {
        let mut out = [b'_'; TEXT_LEN];
        for i in 0..26usize {
            let v = if i < 25 { (self.0 >> (123 - 5 * i)) & 31 } else { (self.0 & 7) << 2 };
            let pos = match i {
                0..=2 => i,
                3..=5 => i + 1,
                _ => i + 2,
            };
            out[pos] = ALPHABET[v as usize];
        }
        out
    }

    /// The system that minted the id.
    pub const fn system(self) -> Code {
        Code((self.0 >> 113) as u16)
    }

    /// The kind.
    pub const fn kind(self) -> Code {
        Code(((self.0 >> 98) & 0x7fff) as u16)
    }

    /// Unix milliseconds stamped by the minter.
    pub const fn time(self) -> u64 {
        ((self.0 >> 48) & TIME_MAX as u128) as u64
    }

    /// The writer number.
    pub const fn writer(self) -> u32 {
        ((self.0 >> 18) & WRITER_MAX as u128) as u32
    }

    /// The count inside the writer's millisecond.
    pub const fn count(self) -> u32 {
        (self.0 & (COUNT_LIMIT as u128 - 1)) as u32
    }

    /// The class of the id's system.
    pub const fn class(self) -> Class {
        self.system().class()
    }

    /// The 128-bit value.
    pub const fn as_u128(self) -> u128 {
        self.0
    }
}

impl fmt::Display for Id {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let t = self.encode();
        f.write_str(std::str::from_utf8(&t).unwrap_or("?"))
    }
}

impl fmt::Debug for Id {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::Display::fmt(self, f)
    }
}
