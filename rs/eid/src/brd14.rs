//! The embedding of brd14 ids into ids of the system `brd` (spec/registry.md#brd14).

use crate::{Code, Id, Refusal, Registry, code};

/// The length of a brd14 id in bytes: a namespace of three bytes `A`–`Z`, then 11 Base62 digits.
pub const BRD14_LEN: usize = 14;
/// The brd14 epoch, 2024-01-01T00:00:00Z, in Unix milliseconds.
pub const BRD14_EPOCH: u64 = 1_704_067_200_000;

const BASE62: &[u8; 62] = b"0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
/// The payload of 2^63 - 1.
const MAX_PAYLOAD: &[u8; 11] = b"AzL8n0Y58m7";
const NOT_BASE62: u8 = 0xff;
const BRD: Code = code(b"brd");

const fn digit_table() -> [u8; 256] {
    let mut table = [NOT_BASE62; 256];
    let mut i = 0;
    while i < 62 {
        table[BASE62[i] as usize] = i as u8;
        i += 1;
    }
    table
}

static DIGIT: [u8; 256] = digit_table();

/// Embeds a brd14 id into an id of the system `brd`: the kind is the namespace in lowercase, the time is
/// [`BRD14_EPOCH`] plus the snowflake's milliseconds, the writer its node and the count its sequence. It reads no
/// registry.
pub fn from_brd14(text: &[u8]) -> Result<Id, Refusal> {
    if text.len() != BRD14_LEN {
        return Err(Refusal::Brd14Length);
    }
    let (ns, payload) = text.split_at(3);
    if !ns.iter().all(u8::is_ascii_uppercase) {
        return Err(Refusal::Brd14Namespace);
    }
    if payload.iter().any(|&c| DIGIT[c as usize] == NOT_BASE62) {
        return Err(Refusal::Brd14Alphabet);
    }
    // Fixed width over an alphabet in ASCII order: byte order is numeric order.
    if payload > &MAX_PAYLOAD[..] {
        return Err(Refusal::Brd14Range);
    }
    let snow = payload.iter().fold(0u64, |s, &c| s * 62 + DIGIT[c as usize] as u64);
    let lower = ns.to_ascii_lowercase();
    let kind = std::str::from_utf8(&lower).ok().and_then(Code::parse).ok_or(Refusal::Brd14Namespace)?;
    Ok(Id::pack(BRD, kind, BRD14_EPOCH + (snow >> 22), ((snow >> 12) & 1023) as u32, (snow & 4095) as u32))
}

/// The inverse of [`from_brd14`]. It reads no registry.
pub fn to_brd14(id: Id) -> Result<String, Refusal> {
    if id.system() != BRD {
        return Err(Refusal::Brd14System);
    }
    let t = id.time();
    if t < BRD14_EPOCH || t - BRD14_EPOCH >= 1 << 41 {
        return Err(Refusal::Brd14Time);
    }
    if id.writer() >= 1 << 10 {
        return Err(Refusal::Brd14Writer);
    }
    if id.count() >= 1 << 12 {
        return Err(Refusal::Brd14Count);
    }
    let mut snow = ((t - BRD14_EPOCH) << 22) | ((id.writer() as u64) << 12) | id.count() as u64;
    let mut out = [0u8; BRD14_LEN];
    for (o, k) in out.iter_mut().zip(id.kind().text()) {
        *o = k.to_ascii_uppercase();
    }
    for o in out[3..].iter_mut().rev() {
        *o = BASE62[(snow % 62) as usize];
        snow /= 62;
    }
    Ok(out.iter().map(|&b| b as char).collect())
}

impl Registry {
    /// The registry layer of [`from_brd14`]: it also refuses a kind that has no row of the system `brd`, in any
    /// state, with [`Refusal::Brd14Kind`].
    pub fn from_brd14(&self, text: &[u8]) -> Result<Id, Refusal> {
        let id = from_brd14(text)?;
        self.kind(BRD, id.kind()).map(|_| id).ok_or(Refusal::Brd14Kind)
    }

    /// The registry layer of [`to_brd14`]: it also refuses a kind that has no row of the system `brd`, in any state,
    /// with [`Refusal::Brd14Kind`].
    pub fn to_brd14(&self, id: Id) -> Result<String, Refusal> {
        let text = to_brd14(id)?;
        self.kind(BRD, id.kind()).map(|_| text).ok_or(Refusal::Brd14Kind)
    }
}
