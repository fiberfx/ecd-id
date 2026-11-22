//! The fence against a cloned allocator store, and the pid guard against a forked minter.

mod common;

use std::cell::Cell;
use std::collections::HashSet;
use std::sync::Arc;
use std::thread;

use common::homed_registry;
use ecd_id::{
    Allocator, Code, Id, LEASE_TERM_MAX, Lease, LeaseStore, ManualClock, MemoryStore, Minter, PidSource, Refusal,
};

const T0: u64 = 1_795_271_400_000;

fn emq() -> Code {
    Code::parse("emq").unwrap()
}

#[test]
fn g7_copy_under_another_home_is_refused() {
    let reg = homed_registry();
    let store = Arc::new(MemoryStore::new());
    let clock = Arc::new(ManualClock::new(T0));
    let _a = Allocator::start(&reg, emq(), "h1", store.clone(), clock.clone(), 1).unwrap();
    let copy: Arc<dyn LeaseStore> = Arc::new(store.snapshot());
    copy.release(emq());
    let refused = Allocator::start(&reg, emq(), "h2", copy, clock.clone(), 1).err();
    assert_eq!(refused, Some(Refusal::NotHome));
    let brd = Code::parse("brd").unwrap();
    let retired = Allocator::start(&reg, brd, "h1", Arc::new(MemoryStore::new()), clock.clone(), 1).err();
    assert_eq!(retired, Some(Refusal::State));
    let detached = Allocator::start(&reg, Code::parse("3ab").unwrap(), "h1", Arc::new(MemoryStore::new()), clock, 1).err();
    assert_eq!(detached, Some(Refusal::Class));
    println!("G7: a copy of the store started under another home is refused; a retired system and a detached code too");
}

#[test]
fn g7_second_activation_is_refused() {
    let reg = homed_registry();
    let store = Arc::new(MemoryStore::new());
    let clock = Arc::new(ManualClock::new(T0));
    let _a = Allocator::start(&reg, emq(), "h1", store.clone(), clock.clone(), 1).unwrap();
    let second = Allocator::start(&reg, emq(), "h1", store.clone(), clock.clone(), 2).err();
    assert_eq!(second, Some(Refusal::Active));
    println!("G7: a second activation of the same code on the same store is refused");
}

/// Two allocators on copies of one store, both believing they are home: the fence is misconfigured. Returns the
/// number of writers both grant.
fn misconfigured_home(offsets: (u32, u32)) -> usize {
    let reg = homed_registry();
    let store = Arc::new(MemoryStore::new());
    let copy = Arc::new(store.snapshot());
    let clock = Arc::new(ManualClock::new(T0));
    let mut a = Allocator::start(&reg, emq(), "h1", store, clock.clone(), offsets.0).unwrap();
    let mut b = Allocator::start(&reg, emq(), "h1", copy, clock.clone(), offsets.1).unwrap();
    let mut wa = HashSet::new();
    let mut shared = 0;
    for _ in 0..20 {
        wa.insert(a.grant(LEASE_TERM_MAX).unwrap().writer);
        if wa.contains(&b.grant(LEASE_TERM_MAX).unwrap().writer) {
            shared += 1;
        }
        clock.advance(1000);
    }
    shared
}

#[test]
fn g7_control_without_the_home_check_both_copies_grant() {
    let same = misconfigured_home((5, 5));
    let different = misconfigured_home((5, 900));
    println!("G7 control (home check bypassed): cloned allocator memory grants {same}/20 shared writers; distinct offsets {different}/20");
    assert_eq!(same, 20);
    assert_eq!(different, 0);
}

thread_local! {
    static PID: Cell<u32> = const { Cell::new(0) };
}

/// The process id of the calling thread, standing for a process: a forked child runs on its own thread with its
/// own pid, holding a copy of its parent's minter.
struct ThreadPid;

impl PidSource for ThreadPid {
    fn pid(&self) -> u32 {
        PID.with(Cell::get)
    }
}

/// Runs a copy of `parent` on its own thread with pid `child_pid`: `n` mints, then a switch to a fresh lease.
fn fork(parent: &Minter, child_pid: u32, n: usize) -> (Vec<Result<Id, Refusal>>, Result<(), Refusal>) {
    let child = parent.clone();
    let h = thread::spawn(move || {
        PID.with(|p| p.set(child_pid));
        let mut c = child;
        let evt = Code::parse("evt").unwrap();
        let out = (0..n).map(|_| c.mint(evt)).collect::<Vec<_>>();
        let next = Lease::new(Code::parse("emq").unwrap(), 9, T0, T0 + LEASE_TERM_MAX).unwrap();
        (out, c.switch(next))
    });
    h.join().unwrap()
}

fn parent_minter(clock: Arc<ManualClock>) -> Minter {
    PID.with(|p| p.set(100));
    let lease = Lease::new(emq(), 4242, T0, T0 + LEASE_TERM_MAX).unwrap();
    Minter::new(lease, clock, Arc::new(ThreadPid))
}

#[test]
fn g7_forked_child_cannot_mint_on_the_parent_lease() {
    let clock = Arc::new(ManualClock::new(T0 + 10));
    let mut parent = parent_minter(clock.clone());
    let evt = Code::parse("evt").unwrap();
    parent.mint(evt).unwrap();
    let (child, switched) = fork(&parent, 101, 5);
    assert!(child.iter().all(|r| *r == Err(Refusal::Fork)));
    assert_eq!(switched, Err(Refusal::Fork));
    assert!(parent.mint(evt).is_ok());
    println!("G7 pid guard: the forked child's 5 mints and its switch are refused; the parent mints on");
}

#[test]
fn g7_control_without_the_pid_guard_parent_and_child_collide() {
    // A child that reports its parent's pid stands for a minter without the guard.
    let clock = Arc::new(ManualClock::new(T0 + 10));
    let parent = parent_minter(clock.clone());
    let mut p = parent.clone();
    let (child, _) = fork(&parent, 100, 1000);
    let evt = Code::parse("evt").unwrap();
    let parent_ids: HashSet<Id> = (0..1000).map(|_| p.mint(evt).unwrap()).collect();
    let dups = child.iter().filter(|r| r.as_ref().is_ok_and(|id| parent_ids.contains(id))).count();
    println!("G7 control (pid guard blind): child and parent mint {dups} equal ids of 1,000");
    assert!(dups > 0);
}
