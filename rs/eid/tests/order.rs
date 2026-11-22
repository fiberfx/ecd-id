//! Text order equals byte order; an unsorted alphabet breaks it.

mod common;

use common::Mix;
use eid::{COUNT_LIMIT, Code, Id, TIME_MAX, WRITER_MAX};

const N: usize = 1_000_000;

fn random_ids(n: usize, seed: u64) -> Vec<Id> {
    let mut g = Mix(seed);
    (0..n)
        .map(|_| {
            let system = Code::from_bits(1024 + g.below((1 << 15) - 1024) as u16).unwrap();
            let k = (6 + g.below(26)) << 10 | (6 + g.below(26)) << 5 | (6 + g.below(26));
            let kind = Code::from_bits(k as u16).unwrap();
            Id::from_parts(
                system,
                kind,
                g.next() & TIME_MAX,
                (g.next() as u32) & WRITER_MAX,
                (g.next() as u32) & (COUNT_LIMIT - 1),
            )
            .unwrap()
        })
        .collect()
}

#[test]
fn g2_text_order_equals_byte_order() {
    let ids = random_ids(N, 0xEC0D_0001);
    let mut by_bytes: Vec<[u8; 16]> = ids.iter().map(|i| i.to_bytes()).collect();
    let mut by_text: Vec<[u8; 28]> = ids.iter().map(|i| i.encode()).collect();
    by_bytes.sort_unstable();
    by_text.sort_unstable();
    for (b, t) in by_bytes.iter().zip(&by_text) {
        let parsed = Id::parse(t).expect("round trip");
        assert_eq!(&parsed.to_bytes(), b);
    }
    println!("G2 good: {N} random ids sort the same as text and as bytes");
}

/// Standard Base32 order (`a-z2-7`), which is not ASCII order.
fn encode_unsorted(id: Id) -> [u8; 28] {
    const STD: &[u8; 32] = b"abcdefghijklmnopqrstuvwxyz234567";
    let sorted = id.encode();
    let mut out = sorted;
    for (i, c) in sorted.iter().enumerate() {
        if *c != b'_' {
            let v = eid::ALPHABET.iter().position(|a| a == c).unwrap();
            out[i] = STD[v];
        }
    }
    out
}

#[test]
fn g2_control_unsorted_alphabet_breaks_order() {
    let ids = random_ids(N, 0xEC0D_0002);
    let mut by_bytes: Vec<Id> = ids.clone();
    by_bytes.sort_unstable();
    let mut by_text: Vec<([u8; 28], Id)> = ids.iter().map(|&i| (encode_unsorted(i), i)).collect();
    by_text.sort_unstable();
    let mismatches = by_bytes.iter().zip(&by_text).filter(|(b, t)| **b != t.1).count();
    println!("G2 control (a-z2-7): {mismatches} of {N} positions differ");
    assert!(mismatches > 0, "the unsorted alphabet should not keep byte order");
}
