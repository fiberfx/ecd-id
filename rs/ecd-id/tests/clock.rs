//! A store restored from an old copy, with the clock right and with the clock set back, while a holder of a lost
//! lease keeps minting.

mod common;

use std::collections::HashSet;
use std::sync::Arc;

use common::{Mix, homed_registry};
use ecd_id::{Allocator, Code, Id, LEASE_TERM_MAX, Lease, LeaseStore, ManualClock, ManualPid, MemoryStore, Minter, Refusal};

const T0: u64 = 1_795_271_400_000;

fn emq() -> Code {
    Code::parse("emq").unwrap()
}

/// Grants one lease per second for `seconds` seconds, waiting out the start second.
fn grant_seconds(a: &mut Allocator, clock: &ManualClock, seconds: u64) -> Vec<Lease> {
    let mut out = Vec::new();
    while out.len() < seconds as usize {
        match a.grant(LEASE_TERM_MAX) {
            Ok(l) => {
                out.push(l);
                clock.advance(1000);
            }
            Err(Refusal::Wait) => clock.advance(1000),
            Err(e) => panic!("grant: {e}"),
        }
    }
    out
}

/// Every holder mints `evt` in the same 200 milliseconds; returns the number of repeated ids.
fn mint_together(leases: &[Lease], from_ms: u64) -> usize {
    let clock = Arc::new(ManualClock::new(from_ms));
    let mut minters: Vec<Minter> =
        leases.iter().map(|l| Minter::new(*l, clock.clone(), Arc::new(ManualPid::new(1)))).collect();
    let evt = Code::parse("evt").unwrap();
    let mut seen: HashSet<Id> = HashSet::new();
    let mut repeats = 0;
    for ms in 0..200 {
        clock.set(from_ms + ms);
        for m in minters.iter_mut() {
            if let Ok(id) = m.mint(evt)
                && !seen.insert(id)
            {
                repeats += 1;
            }
        }
    }
    repeats
}

struct Scenario {
    lost: Vec<Lease>,
    restored: MemoryStore,
    offset: u32,
}

/// An allocator grants for 10 seconds; the store is copied after the 4th grant; 6 more grants follow and are lost
/// when the store is restored from the copy.
fn scenario(offset: u32) -> Scenario {
    let reg = homed_registry();
    let store = Arc::new(MemoryStore::new());
    let clock = Arc::new(ManualClock::new(T0));
    let mut a = Allocator::start(&reg, emq(), "h1", store.clone(), clock.clone(), offset).unwrap();
    let kept = grant_seconds(&mut a, &clock, 4);
    let copy = store.snapshot();
    let lost = grant_seconds(&mut a, &clock, 6);
    assert_eq!(kept.len() + lost.len(), 10);
    copy.release(emq());
    Scenario { lost, restored: copy, offset }
}

#[test]
fn g6_restore_with_a_correct_clock_repeats_nothing() {
    let reg = homed_registry();
    // The worst case for this check: the restarted allocator even draws the same offset.
    let sc = scenario(17);
    let lost_end_second = sc.lost.last().unwrap().not_before / 1000;
    let clock = Arc::new(ManualClock::new((lost_end_second + 3) * 1000 + 250));
    let store: Arc<dyn LeaseStore> = Arc::new(sc.restored);
    let mut b = Allocator::start(&reg, emq(), "h1", store, clock.clone(), sc.offset).unwrap();
    let fresh = grant_seconds(&mut b, &clock, 6);
    let lost_writers: HashSet<u32> = sc.lost.iter().map(|l| l.writer).collect();
    let shared = fresh.iter().filter(|l| lost_writers.contains(&l.writer)).count();
    let mut holders = sc.lost.clone();
    holders.extend(fresh.iter().copied());
    let from = fresh.last().unwrap().not_before;
    let repeats = mint_together(&holders, from);
    println!("G6 good: {} lost leases still minting beside {} fresh ones after a restore with a correct clock: {} shared writers, {} repeated ids", sc.lost.len(), fresh.len(), shared, repeats);
    assert_eq!((shared, repeats), (0, 0));
}

#[test]
fn g6_clock_behind_the_stored_second_is_refused() {
    let reg = homed_registry();
    let store = Arc::new(MemoryStore::new());
    let clock = Arc::new(ManualClock::new(T0));
    let mut a = Allocator::start(&reg, emq(), "h1", store.clone(), clock.clone(), 0).unwrap();
    grant_seconds(&mut a, &clock, 3);
    store.release(emq());
    clock.set(T0 - 5000);
    let mut b = Allocator::start(&reg, emq(), "h1", store.clone(), clock.clone(), 0).unwrap();
    assert_eq!(b.grant(LEASE_TERM_MAX), Err(Refusal::ClockBehind));
    println!("G6: an allocator whose clock is behind the stored grant second refuses");
}

/// Restore plus a clock set back to the first lost grant's second: returns (runs with a repeated writer, repeated ids).
fn restore_and_clock_back(runs: usize, same_offset: bool) -> (usize, usize) {
    let reg = homed_registry();
    let mut g = Mix(0xEC0D_0006);
    let mut runs_with_repeat = 0;
    let mut repeated_ids = 0;
    for _ in 0..runs {
        let offset = (g.next() % 8192) as u32;
        let sc = scenario(offset);
        let first_lost = sc.lost[0].not_before;
        let clock = Arc::new(ManualClock::new(first_lost + 100));
        let store: Arc<dyn LeaseStore> = Arc::new(sc.restored);
        let restart_offset = if same_offset { sc.offset } else { (g.next() % 8192) as u32 };
        let mut b = Allocator::start(&reg, emq(), "h1", store, clock.clone(), restart_offset).unwrap();
        let fresh = grant_seconds(&mut b, &clock, 6);
        let lost_writers: HashSet<u32> = sc.lost.iter().map(|l| l.writer).collect();
        if fresh.iter().any(|l| lost_writers.contains(&l.writer)) {
            runs_with_repeat += 1;
        }
        let mut holders = sc.lost.clone();
        holders.extend(fresh.iter().copied());
        repeated_ids += mint_together(&holders, fresh.last().unwrap().not_before);
    }
    (runs_with_repeat, repeated_ids)
}

#[test]
fn g6_violating_restore_and_clock_back_repeats_are_counted() {
    let (rolled, rolled_ids) = restore_and_clock_back(20, true);
    let (fresh, fresh_ids) = restore_and_clock_back(20, false);
    println!("G6 COUNT restore + clock set back, allocator memory rolled back too (same offset): {rolled}/20 runs repeat writers, {rolled_ids} repeated ids");
    println!("G6 COUNT restore + clock set back, allocator restarted (fresh random offset): {fresh}/20 runs repeat writers, {fresh_ids} repeated ids");
    assert_eq!(rolled, 20, "with memory and clock rolled back the lost grants repeat exactly");
    assert!(rolled_ids > 0);
}
