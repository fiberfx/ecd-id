//! The door: detached ids bound as command tokens, never stored as the system's own; foreign, ahead and
//! out-of-lease ids refused.

mod common;

use std::collections::HashSet;
use std::sync::Arc;

use eid::{
    Admission, Class, Code, DEFAULT_AHEAD_MS, Door, Id, LEASE_TERM_MAX, Lease, ManualClock, ManualPid, Minter, OsRandom,
    Refusal, detached_lease,
};

const T: u64 = 1_795_271_400_000;
const AHEAD_MS: u64 = 2_000;

fn code(s: &str) -> Code {
    Code::parse(s).unwrap()
}

fn minter(system: &str, writer: u32, clock: Arc<ManualClock>) -> Minter {
    Minter::new(Lease::new(code(system), writer, T, T + LEASE_TERM_MAX).unwrap(), clock, Arc::new(ManualPid::new(1)))
}

fn door(clock: Arc<ManualClock>) -> Door {
    Door::new(minter("emq", 1, clock.clone()), &[code("job"), code("evt")], AHEAD_MS, clock)
}

/// Producers: one with a lease from emq (writer 7), and 50 detached ones. Returns their ids, in mint order.
fn traffic(clock: Arc<ManualClock>) -> (Vec<Id>, Lease) {
    let mut leased = minter("emq", 7, clock.clone());
    let lease = leased.lease();
    let mut ids = Vec::new();
    let job = code("job");
    for p in 0..50u32 {
        let mut d = minter(&format!("3a{}", eid::ALPHABET[(p % 32) as usize] as char), 100 + p, clock.clone());
        for _ in 0..4 {
            ids.push(d.mint(job).unwrap());
            ids.push(leased.mint(job).unwrap());
            clock.advance(1);
        }
    }
    (ids, lease)
}

#[test]
fn g10_detached_ids_are_bound_never_stored_as_own() {
    let clock = Arc::new(ManualClock::new(T + 5));
    let mut d = door(clock.clone());
    let (ids, lease) = traffic(clock.clone());
    let mut own_facts: Vec<Id> = Vec::new();
    let mut bound = 0;
    for id in &ids {
        match d.admit(*id, if id.class() == Class::Detached { None } else { Some(&lease) }).unwrap() {
            Admission::Own(x) => own_facts.push(x),
            Admission::Bound { token, id: own } => {
                assert_eq!(token, *id);
                assert_eq!(own.system(), code("emq"));
                assert_eq!(own.kind(), id.kind());
                own_facts.push(own);
                bound += 1;
            }
        }
    }
    let detached_stored = own_facts.iter().filter(|x| x.class() == Class::Detached).count();
    println!("G10 good: {} ids admitted, {bound} detached ids bound as command tokens, {detached_stored} detached ids stored as own", ids.len());
    assert_eq!(detached_stored, 0);
    assert_eq!(bound, 200);
    assert!(own_facts.iter().all(|x| x.system() == code("emq")));
}

#[test]
fn g10_admitting_a_detached_id_again_returns_the_same_binding() {
    let clock = Arc::new(ManualClock::new(T + 5));
    let mut d = door(clock.clone());
    let lease = detached_lease(&OsRandom, clock.as_ref(), None).expect("random source");
    assert_eq!(lease.system.class(), Class::Detached);
    let mut producer = Minter::new(lease, clock.clone(), Arc::new(ManualPid::new(1)));
    let token = producer.mint(code("job")).unwrap();
    let first = d.admit(token, None).unwrap();
    let again = d.admit(token, None).unwrap();
    assert_eq!(first, again);
    assert_eq!(d.bound(), 1);
    println!("G10: a retried detached id {token} binds to the same id: {first:?}");
}

#[test]
fn g10_control_without_the_door_detached_ids_become_facts() {
    let clock = Arc::new(ManualClock::new(T + 5));
    let (ids, _) = traffic(clock);
    // With no door, the store keeps whatever arrives.
    let stored: HashSet<Id> = ids.iter().copied().collect();
    let detached_stored = stored.iter().filter(|x| x.class() == Class::Detached).count();
    println!("G10 control (door off): {detached_stored} of {} stored facts are detached ids that name no system", stored.len());
    assert!(detached_stored > 0);
}

#[test]
fn g10_foreign_and_out_of_lease_ids_refused() {
    let clock = Arc::new(ManualClock::new(T + 5));
    let mut d = door(clock.clone());
    let foreign = minter("kan", 7, clock.clone()).mint(code("job")).unwrap();
    assert_eq!(d.admit(foreign, None), Err(Refusal::Foreign));
    let mut m = minter("emq", 7, clock.clone());
    let lease = m.lease();
    let other_writer = Lease::new(code("emq"), 8, T, T + LEASE_TERM_MAX).unwrap();
    let id = m.mint(code("job")).unwrap();
    assert_eq!(d.admit(id, Some(&other_writer)), Err(Refusal::Lease));
    assert_eq!(d.admit(id, Some(&lease)), Ok(Admission::Own(id)));
    println!("G10: an id of another system and an id outside the session's lease are refused");
}

#[test]
fn g4_door_refuses_an_id_stamped_ahead_of_its_clock() {
    let door_clock = Arc::new(ManualClock::new(T + 5));
    let mut d = door(door_clock.clone());
    let fast = Arc::new(ManualClock::new(T + 5 + AHEAD_MS + 1));
    let ahead = minter("emq", 7, fast.clone()).mint(code("evt")).unwrap();
    assert_eq!(d.admit(ahead, None), Err(Refusal::Ahead));
    fast.set(T + 5 + AHEAD_MS);
    let edge = minter("emq", 7, fast).mint(code("evt")).unwrap();
    assert_eq!(d.admit(edge, None), Ok(Admission::Own(edge)));
    // The default limit.
    let mut dd = Door::new(minter("emq", 1, door_clock.clone()), &[code("evt")], DEFAULT_AHEAD_MS, door_clock.clone());
    let at = |ms: u64| minter("emq", 7, Arc::new(ManualClock::new(ms))).mint(code("evt")).unwrap();
    assert_eq!(dd.admit(at(T + 5 + DEFAULT_AHEAD_MS + 1), None), Err(Refusal::Ahead));
    let ok = at(T + 5 + DEFAULT_AHEAD_MS);
    assert_eq!(dd.admit(ok, None), Ok(Admission::Own(ok)));
    println!(
        "G4 door: an id {} ms ahead is refused; {} ms ahead is admitted; with the default limit, {} refused and {} admitted",
        AHEAD_MS + 1,
        AHEAD_MS,
        DEFAULT_AHEAD_MS + 1,
        DEFAULT_AHEAD_MS
    );
}

#[test]
fn g4_door_refuses_a_foreign_kind() {
    let clock = Arc::new(ManualClock::new(T + 5));
    let mut d = door(clock.clone());
    let msg = minter("emq", 7, clock.clone()).mint(code("msg")).unwrap();
    assert_eq!(d.admit(msg, None), Err(Refusal::Kind));
    let detached_msg = minter("3ab", 9, clock).mint(code("msg")).unwrap();
    assert_eq!(d.admit(detached_msg, None), Err(Refusal::Kind));
    println!("G4 door: a kind the stream does not carry is refused, detached or not");
}
