//! brd14 ids under the system `brd` (spec/embed.md).

use crate::{Code, Id, Refusal, code};

/// The system that holds brd14 embeddings.
pub const BRD: Code = code(b"brd");
/// brd14's epoch: 2024-01-01T00:00:00Z in Unix milliseconds.
pub const BRD14_EPOCH: u64 = 1_704_067_200_000;

const BASE62: &[u8; 62] = b"0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
const MAX_PAYLOAD: &[u8; 11] = b"AzL8n0Y58m7";

fn base62_value(c: u8) -> Option<u64> {
    match c {
        b'0'..=b'9' => Some((c - b'0') as u64),
        b'A'..=b'Z' => Some((c - b'A') as u64 + 10),
        b'a'..=b'z' => Some((c - b'a') as u64 + 36),
        _ => None,
    }
}

/// Decodes a brd14 id into its namespace and 63-bit number, refusing in brd14's order.
pub fn brd14_decode(t: &[u8]) -> Result<([u8; 3], u64), Refusal> {
    if t.len() != 14 {
        return Err(Refusal::Brd14Length);
    }
    if !t[..3].iter().all(u8::is_ascii_uppercase) {
        return Err(Refusal::Brd14Namespace);
    }
    let payload = &t[3..];
    let mut snow: u64 = 0;
    for &c in payload {
        let v = base62_value(c).ok_or(Refusal::Brd14Charset)?;
        snow = snow.wrapping_mul(62).wrapping_add(v);
    }
    if payload > &MAX_PAYLOAD[..] {
        return Err(Refusal::Brd14Range);
    }
    Ok(([t[0], t[1], t[2]], snow))
}

/// Encodes a namespace and a number below 2^63 as a brd14 id.
pub fn brd14_encode(ns: [u8; 3], mut snow: u64) -> String {
    let mut out = [b'0'; 14];
    out[..3].copy_from_slice(&ns);
    for i in (3..14).rev() {
        out[i] = BASE62[(snow % 62) as usize];
        snow /= 62;
    }
    String::from_utf8_lossy(&out).into_owned()
}

/// The embedding of a brd14 id: system `brd`, the namespace in lowercase, and the number's fields.
pub fn embed(t: &[u8]) -> Result<Id, Refusal> {
    let (ns, snow) = brd14_decode(t)?;
    let lower = [ns[0].to_ascii_lowercase(), ns[1].to_ascii_lowercase(), ns[2].to_ascii_lowercase()];
    let kind = Code::parse(std::str::from_utf8(&lower).map_err(|_| Refusal::Brd14Namespace)?)
        .ok_or(Refusal::Brd14Namespace)?;
    Ok(Id::pack(BRD, kind, (snow >> 22) + BRD14_EPOCH, ((snow >> 12) & 1023) as u32, (snow & 4095) as u32))
}

/// The brd14 id that an embedding came from.
pub fn unembed(id: Id) -> Result<String, Refusal> {
    let t = id.time().checked_sub(BRD14_EPOCH).ok_or(Refusal::NotEmbedding)?;
    if id.system() != BRD || t >= 1 << 41 || id.writer() >= 1024 || id.count() >= 4096 {
        return Err(Refusal::NotEmbedding);
    }
    let k = id.kind().text();
    let ns = [k[0].to_ascii_uppercase(), k[1].to_ascii_uppercase(), k[2].to_ascii_uppercase()];
    Ok(brd14_encode(ns, (t << 22) | ((id.writer() as u64) << 12) | id.count() as u64))
}
