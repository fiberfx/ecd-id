//! Lease-free ids are checked by kind only (spec/registry.md#lease-free): the registry is consulted for registered
//! systems, not for detached ones.

mod common;

use std::sync::Arc;

use common::repo_registry;
use eid::{Class, Code, Door, Id, LEASE_TERM_MAX, Lease, ManualClock, ManualPid, Minter, Refusal};

const T: u64 = 1_795_271_400_000;

fn code(s: &str) -> Code {
    Code::parse(s).unwrap()
}

fn minter(system: &str, writer: u32, clock: Arc<ManualClock>) -> Minter {
    Minter::new(Lease::new(code(system), writer, T, T + LEASE_TERM_MAX).unwrap(), clock, Arc::new(ManualPid::new(1)))
}

#[test]
fn registry_ecd_owner() {
    let reg = repo_registry();
    let row = reg.system(code("ecd")).expect("ecd row");
    assert_eq!(row.owner, "ecd-core");
    assert_eq!(row.home, "-");
}

#[test]
fn registry_lease_free_kind_named_by_no_row_passes() {
    let reg = repo_registry();
    assert!(reg.system(code("3ab")).is_none());
    let id = minter("3ab", 123_456_789, Arc::new(ManualClock::new(T + 5))).mint_kind("zzz").expect("mint");
    let back = Id::parse(id.to_string().as_bytes()).expect("parse");
    assert_eq!((back, back.class(), back.kind()), (id, Class::Detached, code("zzz")));
    println!("registry: detached {id} passes with no row for system 3ab or kind zzz");
}

#[test]
fn registry_lease_free_kind_with_a_digit_is_refused() {
    let mut m = minter("3ab", 123_456_789, Arc::new(ManualClock::new(T + 5)));
    assert_eq!(m.mint_kind("j2b"), Err(Refusal::Kind));
    assert_eq!(Id::parse(b"3ab_j2b_25rgqgbq225prncp2222"), Err(Refusal::Kind));
    println!("registry: detached kind j2b refused: kind");
}

#[test]
fn registry_registered_kind_without_a_row_is_refused() {
    let reg = repo_registry();
    let emq = code("emq");
    assert!(reg.kind(emq, code("zzz")).is_none());
    assert!(reg.kind(emq, code("job")).is_some());
    let clock = Arc::new(ManualClock::new(T + 5));
    let kinds: Vec<Code> = reg.kinds_of(emq).map(|k| k.kind).collect();
    let mut door = Door::new(minter("emq", 1, clock.clone()), &kinds, 2_000, clock.clone());
    let own = minter("emq", 7, clock.clone()).mint(code("zzz")).expect("mint");
    assert_eq!(door.admit(own, None), Err(Refusal::Kind));
    // The same kind name stands detached: it is compared by name against the door's kinds, and the registry is not
    // consulted for system 3ab.
    let mut det = minter("3ab", 123_456_789, clock);
    let zzz = det.mint_kind("zzz").expect("mint");
    assert_eq!(door.admit(zzz, None), Err(Refusal::Kind));
    let job = det.mint_kind("job").expect("mint");
    match door.admit(job, None).expect("admit") {
        eid::Admission::Bound { id, .. } => assert_eq!((id.system(), id.kind()), (emq, code("job"))),
        other => panic!("detached id of an accepted kind: {other:?}"),
    }
    println!("registry: emq/zzz refused: kind; detached zzz refused by name at the door; detached job bound");
}
