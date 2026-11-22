//! The Valkey stream-id mapping and its inverse (spec/stream.md).

use std::fmt;

use crate::{COUNT_LIMIT, Class, Code, Id, Refusal, TIME_MAX, WRITER_MAX};

/// A Valkey stream entry id: two unsigned 64-bit numbers, ordered by `ms`, then `seq`.
#[derive(Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord, Debug)]
pub struct StreamId {
    /// The millisecond part.
    pub ms: u64,
    /// The sequence part.
    pub seq: u64,
}

impl StreamId {
    /// Parses `<ms>-<seq>`: two decimal numbers below 2^64, without sign or leading zero, joined by one `-`.
    pub fn parse(s: &str) -> Result<StreamId, Refusal> {
        let (a, b) = s.split_once('-').ok_or(Refusal::StreamSyntax)?;
        Ok(StreamId { ms: decimal(a)?, seq: decimal(b)? })
    }
}

fn decimal(p: &str) -> Result<u64, Refusal> {
    let b = p.as_bytes();
    if b.is_empty() || !b.iter().all(u8::is_ascii_digit) || (b.len() > 1 && b[0] == b'0') {
        return Err(Refusal::StreamSyntax);
    }
    p.parse::<u64>().map_err(|_| Refusal::StreamSyntax)
}

impl fmt::Display for StreamId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}-{}", self.ms, self.seq)
    }
}

impl Id {
    /// The stream entry id: `time`, then `system << 48 | writer << 18 | count`.
    pub fn stream_id(self) -> StreamId {
        StreamId {
            ms: self.time(),
            seq: ((self.system().bits() as u64) << 48) | ((self.writer() as u64) << 18) | self.count() as u64,
        }
    }

    /// The 48-bit tail without the system (bytes 10–15). It is not unique across systems; it exists only as the
    /// control that shows why the stream id carries the system.
    pub fn legacy_tail(self) -> u64 {
        ((self.writer() as u64) << 18) | self.count() as u64
    }
}

/// Rebuilds an id from a stream entry id and the kind that the stream carries.
pub fn from_stream(kind: &str, sid: StreamId) -> Result<Id, Refusal> {
    let kind = Code::parse(kind).filter(|k| k.is_kind()).ok_or(Refusal::Kind)?;
    if sid.ms > TIME_MAX || sid.seq >= 1 << 63 {
        return Err(Refusal::StreamRange);
    }
    let system = Code::from_bits((sid.seq >> 48) as u16).ok_or(Refusal::StreamRange)?;
    if system.class() == Class::Reserved {
        return Err(Refusal::SystemReserved);
    }
    Ok(Id::pack(
        system,
        kind,
        sid.ms,
        ((sid.seq >> 18) & WRITER_MAX as u64) as u32,
        (sid.seq & (COUNT_LIMIT as u64 - 1)) as u32,
    ))
}

/// Valkey's rule for `XADD` with an explicit id: the id is greater than the stream's top entry and is not `0-0`.
pub fn valkey_accepts(top: Option<StreamId>, entry: StreamId) -> bool {
    if entry == (StreamId { ms: 0, seq: 0 }) {
        return false;
    }
    match top {
        None => true,
        Some(t) => entry > t,
    }
}
