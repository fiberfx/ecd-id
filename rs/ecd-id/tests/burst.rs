//! Bursts: eight minters on eight leases, borrowing past 262,144 ids in one millisecond, two kinds on one minter,
//! and the control of two minters on one lease.

mod common;

use std::collections::HashSet;
use std::sync::Arc;
use std::thread;

use ecd_id::{COUNT_LIMIT, Code, Id, Lease, ManualClock, ManualPid, Minter};

const T: u64 = 1_795_271_400_123;
const TOTAL: usize = 10_000_000;

fn emq() -> Code {
    Code::parse("emq").unwrap()
}

fn lease(writer: u32) -> Lease {
    Lease::new(emq(), writer, T - 123, T - 123 + 86_400_000).unwrap()
}

#[test]
fn g3_eight_leases_ten_million_ids() {
    // A frozen clock: every minter stamps far past 262,144 ids in its first millisecond and keeps borrowing.
    let clock = Arc::new(ManualClock::new(T));
    let per = TOTAL / 8;
    let handles: Vec<_> = (0..8u32)
        .map(|w| {
            let clock = clock.clone();
            thread::spawn(move || {
                let mut m = Minter::new(lease(1000 + w), clock, Arc::new(ManualPid::new(7)));
                let kind = Code::parse("evt").unwrap();
                let mut out = Vec::with_capacity(per);
                for _ in 0..per {
                    out.push(m.mint(kind).expect("mint").as_u128());
                }
                out
            })
        })
        .collect();
    let mut all = Vec::with_capacity(TOTAL);
    for h in handles {
        let ids = h.join().unwrap();
        assert!(ids.windows(2).all(|w| w[0] < w[1]), "a writer's ids must rise strictly");
        let last = Id::from_bytes(&ids[ids.len() - 1].to_be_bytes()).unwrap();
        assert_eq!(last.time(), T + (per as u64 - 1) / COUNT_LIMIT as u64);
        all.extend(ids);
    }
    all.sort_unstable();
    let before = all.len();
    all.dedup();
    println!("G3 good: {before} ids from 8 leases, {} distinct; each writer strictly rising", all.len());
    assert_eq!(all.len(), TOTAL);
}

#[test]
fn g3_borrows_past_the_count_limit() {
    let clock = Arc::new(ManualClock::new(T));
    let mut m = Minter::new(lease(5), clock, Arc::new(ManualPid::new(7)));
    let kind = Code::parse("job").unwrap();
    let mut last = None;
    for i in 0..COUNT_LIMIT {
        let id = m.mint(kind).unwrap();
        assert_eq!((id.time(), id.count()), (T, i));
        last = Some(id);
    }
    let next = m.mint(kind).unwrap();
    assert_eq!((next.time(), next.count()), (T + 1, 0));
    assert!(next > last.unwrap());
    println!("G3 borrow: id 262,145 in one millisecond is stamped {} count 0", next.time());
}

#[test]
fn g3_two_kinds_on_one_minter_never_repeat_a_time_count_pair() {
    let clock = Arc::new(ManualClock::new(T));
    let mut m = Minter::new(lease(9), clock.clone(), Arc::new(ManualPid::new(7)));
    let (a, b) = (Code::parse("job").unwrap(), Code::parse("evt").unwrap());
    let mut pairs = HashSet::new();
    for i in 0..600_000u64 {
        if i % 1000 == 0 {
            clock.advance(1);
        }
        let id = m.mint(if i % 2 == 0 { a } else { b }).unwrap();
        assert!(pairs.insert((id.time(), id.count())), "repeated (time, count) at {i}");
    }
    println!("G3 two kinds: 600,000 ids alternating job and evt, no (time, count) pair repeats");
}

#[test]
fn g3_control_two_minters_on_one_lease_duplicate() {
    let clock = Arc::new(ManualClock::new(T));
    let kind = Code::parse("evt").unwrap();
    let mut m1 = Minter::new(lease(77), clock.clone(), Arc::new(ManualPid::new(7)));
    let mut m2 = Minter::new(lease(77), clock.clone(), Arc::new(ManualPid::new(7)));
    let mut seen = HashSet::new();
    let mut dups = 0;
    for _ in 0..1000 {
        for m in [&mut m1, &mut m2] {
            if !seen.insert(m.mint(kind).unwrap()) {
                dups += 1;
            }
        }
    }
    println!("G3 control (one lease, two minters): {dups} duplicates in 2,000 ids");
    assert!(dups > 0, "two minters on one lease must collide: this is why P3 is a premise");
}
