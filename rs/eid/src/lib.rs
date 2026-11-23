//! The Ecd ID, version 1.
//!
//! A 16-byte identifier: system (15 bits), kind (15), reserved (2), Unix milliseconds (48), writer (30) and count
//! (18). Its canonical text is 28 lowercase characters such as `emq_job_23obzcbapvupu2272222`. The specification is
//! in `spec/` at the repository root; the vectors in `vectors/` are the conformance contract.
//!
//! The crate uses the standard library only. Clocks, lease stores, process ids and random sources are traits, so
//! restarts, clocks set back and cloned processes can be exercised deterministically.

#![forbid(unsafe_code)]

mod brd14;
mod door;
mod id;
mod lease;
mod mint;
mod registry;
mod stream;

use std::fmt;

pub use brd14::{BRD14_EPOCH, BRD14_LEN, from_brd14, to_brd14};
pub use door::{Admission, DEFAULT_AHEAD_MS, Door};
pub use id::{ALPHABET, COUNT_LIMIT, Class, Code, Id, SYS, TEXT_LEN, TIME_MAX, WRITER_MAX, code};
pub use lease::{
    Allocator, Clock, INDEX_SLOTS, LEASE_TERM_MAX, Lease, LeaseStore, ManualClock, MemoryStore, SECOND_SLOTS,
    SystemClock, writer_for,
};
pub use mint::{ManualPid, Minter, OsRandom, PidSource, ProcessPid, RandomSource, detached_lease};
pub use registry::{KindRow, Registry, RegistryError, Role, State, SystemRow};
pub use stream::{StreamId, from_stream, valkey_accepts};

/// A refusal, with the reason named in the specification and the vectors.
#[derive(Clone, Copy, PartialEq, Eq, Debug, Hash)]
pub enum Refusal {
    /// The text is not 28 bytes, or the binary input is not 16 bytes.
    Length,
    /// Byte 3 or byte 7 of the text is not `_`.
    Separator,
    /// A byte of the text is outside the alphabet.
    Alphabet,
    /// The two pad bits of the last character are not zero.
    Pad,
    /// The two reserved bits are not zero.
    Reserved,
    /// The system code is of the reserved class.
    SystemReserved,
    /// The kind is not three letters.
    Kind,
    /// A field is outside its width.
    Range,
    /// A stream id text is not `<ms>-<seq>` in canonical decimal.
    StreamSyntax,
    /// A stream id is outside the range of the mapping.
    StreamRange,
    /// A lease text is malformed.
    LeaseSyntax,
    /// A lease names a reserved system, or a switch names another system.
    LeaseSystem,
    /// A lease's writer is outside 30 bits.
    LeaseWriter,
    /// A lease's window is empty, starts at zero, ends past 2^48 or exceeds 24 hours.
    LeaseWindow,
    /// The minter would stamp past the end of its lease.
    LeaseEnded,
    /// The minter runs under another process id than the one it was created under.
    Fork,
    /// The random source failed.
    Random,
    /// The allocator has just started in the second of the stored grant: it waits for the next second.
    Wait,
    /// The clock is behind the stored grant second.
    ClockBehind,
    /// 8,192 grants were made in this second.
    SecondFull,
    /// The stored grant second changed under this allocator.
    Contended,
    /// The clock reads second zero.
    ClockZero,
    /// The requested lease term is zero or longer than 24 hours.
    Term,
    /// An allocator was asked for a system that is not of the registered class.
    Class,
    /// The system has no registry row.
    Unknown,
    /// The system's registry row is not live.
    State,
    /// This supervisor is not the system's home.
    NotHome,
    /// Another activation of the system is live in the store.
    Active,
    /// The door: the id's system is not the door's system.
    Foreign,
    /// The door: the id is stamped too far ahead of the door's clock.
    Ahead,
    /// The door: the id is outside the lease the session holds.
    Lease,
    /// A brd14 id is not 14 bytes.
    Brd14Length,
    /// A byte of a brd14 id's namespace is outside `A`–`Z`.
    Brd14Namespace,
    /// A byte of a brd14 id's payload is outside the Base62 alphabet.
    Brd14Alphabet,
    /// A brd14 id's payload is 2^63 or more.
    Brd14Range,
    /// The id's system is not `brd`.
    Brd14System,
    /// The id's time is outside the brd14 epoch's 41 bits.
    Brd14Time,
    /// The id's writer is 2^10 or more.
    Brd14Writer,
    /// The id's count is 2^12 or more.
    Brd14Count,
    /// The kind has no row of the system `brd` in the registry.
    Brd14Kind,
}

impl Refusal {
    /// The reason as the specification and the vectors name it.
    pub const fn reason(self) -> &'static str {
        match self {
            Refusal::Length => "length",
            Refusal::Separator => "separator",
            Refusal::Alphabet => "alphabet",
            Refusal::Pad => "pad",
            Refusal::Reserved => "reserved",
            Refusal::SystemReserved => "system-reserved",
            Refusal::Kind => "kind",
            Refusal::Range => "range",
            Refusal::StreamSyntax => "stream-syntax",
            Refusal::StreamRange => "stream-range",
            Refusal::LeaseSyntax => "lease-syntax",
            Refusal::LeaseSystem => "lease-system",
            Refusal::LeaseWriter => "lease-writer",
            Refusal::LeaseWindow => "lease-window",
            Refusal::LeaseEnded => "lease-ended",
            Refusal::Fork => "fork",
            Refusal::Random => "random",
            Refusal::Wait => "wait",
            Refusal::ClockBehind => "clock-behind",
            Refusal::SecondFull => "second-full",
            Refusal::Contended => "contended",
            Refusal::ClockZero => "clock-zero",
            Refusal::Term => "term",
            Refusal::Class => "class",
            Refusal::Unknown => "unknown",
            Refusal::State => "state",
            Refusal::NotHome => "not-home",
            Refusal::Active => "active",
            Refusal::Foreign => "foreign",
            Refusal::Ahead => "ahead",
            Refusal::Lease => "lease",
            Refusal::Brd14Length => "brd14-length",
            Refusal::Brd14Namespace => "brd14-namespace",
            Refusal::Brd14Alphabet => "brd14-alphabet",
            Refusal::Brd14Range => "brd14-range",
            Refusal::Brd14System => "brd14-system",
            Refusal::Brd14Time => "brd14-time",
            Refusal::Brd14Writer => "brd14-writer",
            Refusal::Brd14Count => "brd14-count",
            Refusal::Brd14Kind => "brd14-kind",
        }
    }
}

impl fmt::Display for Refusal {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.reason())
    }
}

impl std::error::Error for Refusal {}
