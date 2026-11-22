//! Stream ids across systems on one Valkey stream, checked against Valkey's documented XADD rule.

mod common;

use std::collections::HashSet;
use std::sync::Arc;

use ecd_id::{Code, Id, Lease, ManualClock, ManualPid, Minter, StreamId, from_stream, valkey_accepts};

pub const T: u64 = 1_795_357_800_000;
const WRITER: u32 = 3 << 13;

fn minter(system: &str, clock: Arc<ManualClock>) -> Minter {
    let lease = Lease::new(Code::parse(system).unwrap(), WRITER, T - 5000, T - 5000 + 86_400_000).unwrap();
    Minter::new(lease, clock, Arc::new(ManualPid::new(1)))
}

/// Two systems forced onto the same writer, both minting one `evt` in each of `ms` milliseconds.
fn two_systems(ms: u64) -> Vec<Id> {
    let clock = Arc::new(ManualClock::new(T));
    let (mut a, mut b) = (minter("emq", clock.clone()), minter("kan", clock.clone()));
    let evt = Code::parse("evt").unwrap();
    let mut ids = Vec::new();
    for i in 0..ms {
        clock.set(T + i);
        ids.push(a.mint(evt).unwrap());
        ids.push(b.mint(evt).unwrap());
    }
    ids
}

/// Appends in order under Valkey's rule; returns how many entries were refused.
fn append_all(entries: &[StreamId]) -> usize {
    let mut top = None;
    let mut refused = 0;
    for &e in entries {
        if valkey_accepts(top, e) {
            top = Some(e);
        } else {
            refused += 1;
        }
    }
    refused
}

#[test]
fn g4_two_systems_same_writer_same_ms_distinct_and_ordered() {
    let ids = two_systems(1000);
    let mut sids: Vec<StreamId> = ids.iter().map(|i| i.stream_id()).collect();
    let distinct: HashSet<StreamId> = sids.iter().copied().collect();
    assert_eq!(distinct.len(), ids.len());
    sids.sort();
    assert!(sids.windows(2).all(|w| w[0].ms <= w[1].ms));
    assert_eq!(append_all(&sids), 0);
    for (sid, id) in sids.iter().zip({
        let mut v = ids.clone();
        v.sort_by_key(|i| i.stream_id());
        v
    }) {
        assert_eq!(from_stream("evt", *sid).unwrap(), id);
    }
    println!("G4 good: {} ids from 2 systems on writer {WRITER}, same ms: {} distinct stream ids, appended in ms order, inverse exact", ids.len(), distinct.len());
}

#[test]
fn g4_control_legacy_tail_collides() {
    let ids = two_systems(1000);
    let legacy: Vec<StreamId> = ids.iter().map(|i| StreamId { ms: i.time(), seq: i.legacy_tail() }).collect();
    let distinct: HashSet<StreamId> = legacy.iter().copied().collect();
    let mut sorted = legacy.clone();
    sorted.sort();
    let refused = append_all(&sorted);
    println!("G4 control (48-bit tail without the system): {} ids, {} distinct stream ids, {} appends refused", ids.len(), distinct.len(), refused);
    assert_eq!(distinct.len(), ids.len() / 2);
    assert_eq!(refused, ids.len() / 2);
}

#[test]
fn g4_refused_same_ms_append_succeeds_when_reminted() {
    let clock = Arc::new(ManualClock::new(T));
    let (mut low, mut high) = (minter("emq", clock.clone()), minter("kan", clock.clone()));
    let evt = Code::parse("evt").unwrap();
    let h = high.mint(evt).unwrap().stream_id();
    let l = low.mint(evt).unwrap().stream_id();
    let mut top = None;
    assert!(valkey_accepts(top, h));
    top = Some(h);
    assert!(!valkey_accepts(top, l), "the lower tail in the same ms loses the race");
    clock.advance(1);
    let again = low.mint(evt).unwrap().stream_id();
    assert!(valkey_accepts(top, again));
    println!("G4 race: {l} refused after {h}; re-minted as {again}, accepted");
}

#[test]
fn g4_valkey_rule_refuses_zero_and_equal() {
    let z = StreamId { ms: 0, seq: 0 };
    assert!(!valkey_accepts(None, z));
    let a = StreamId { ms: 5, seq: 1 };
    assert!(!valkey_accepts(Some(a), a));
    assert!(valkey_accepts(Some(a), StreamId { ms: 5, seq: 2 }));
    assert!(valkey_accepts(Some(a), StreamId { ms: 6, seq: 0 }));
    assert!(!valkey_accepts(Some(a), StreamId { ms: 4, seq: u64::MAX }));
}
