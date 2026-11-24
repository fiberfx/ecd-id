//! The detached code draw of spec/lease.md#detached: one 16-bit draw r; r >= 61440 is drawn again, at most 64 times;
//! the code is 1024 + r mod 5120.

use std::cell::Cell;

use eid::{Class, Code, ManualClock, RandomSource, Refusal, detached_lease};

const T: u64 = 1_795_271_400_000;

/// A random source that returns given bytes in order; a read past the end fails with `Refusal::Random`.
struct Scripted {
    data: Vec<u8>,
    pos: Cell<usize>,
}

impl Scripted {
    fn new(data: Vec<u8>) -> Scripted {
        Scripted { data, pos: Cell::new(0) }
    }
}

impl RandomSource for Scripted {
    fn fill(&self, buf: &mut [u8]) -> Result<(), Refusal> {
        let pos = self.pos.get();
        if pos + buf.len() > self.data.len() {
            return Err(Refusal::Random);
        }
        buf.copy_from_slice(&self.data[pos..pos + buf.len()]);
        self.pos.set(pos + buf.len());
        Ok(())
    }
}

/// The bytes of the given 16-bit draws followed by the four writer bytes 00 00 00 07.
fn draw_bytes(draws: &[u16]) -> Vec<u8> {
    let mut b: Vec<u8> = draws.iter().flat_map(|d| d.to_be_bytes()).collect();
    b.extend_from_slice(&[0, 0, 0, 7]);
    b
}

fn lease_of(src: &Scripted) -> eid::Lease {
    let lease = detached_lease(src, &ManualClock::new(T), None).expect("detached lease");
    assert_eq!(lease.system.class(), Class::Detached);
    assert_eq!(lease.writer, 7);
    lease
}

#[test]
fn detached_draw_one_rejection() {
    let src = Scripted::new(vec![0xF0, 0x00, 0x00, 0x00, 0, 0, 0, 7]);
    let lease = lease_of(&src);
    assert_eq!(lease.system.to_string(), "322");
    assert_eq!(lease.system.bits(), 1024);
    assert_eq!(src.pos.get(), src.data.len());
}

#[test]
fn detached_draw_last_accepted() {
    let src = Scripted::new(vec![0xEF, 0xFF, 0, 0, 0, 7]);
    let lease = lease_of(&src);
    assert_eq!(lease.system.to_string(), "7zz");
    assert_eq!(lease.system.bits(), 6143);
}

#[test]
fn detached_draw_after_sixty_three_rejections() {
    let mut draws = vec![0xFFFF; 63];
    draws.push(5);
    let lease = lease_of(&Scripted::new(draw_bytes(&draws)));
    assert_eq!(lease.system.to_string(), "327");
    assert_eq!(lease.system.bits(), 1029);
}

#[test]
fn detached_draw_sixty_four_rejections_refused() {
    let src = Scripted::new(draw_bytes(&[0xFFFF; 64]));
    let err = detached_lease(&src, &ManualClock::new(T), None).unwrap_err();
    assert_eq!(err, Refusal::Random);
    assert_eq!(src.pos.get(), 128, "64 draws and no writer draw");
}

#[test]
fn detached_draw_source_failure_returns_at_once() {
    let src = Scripted::new(vec![0xFF]);
    let err = detached_lease(&src, &ManualClock::new(T), None).unwrap_err();
    assert_eq!(err, Refusal::Random);
    assert_eq!(src.pos.get(), 0);
}

#[test]
fn detached_draw_every_code_twelve_times() {
    let mut seen = vec![0u32; 1 << 15];
    for r in 0..61440u32 {
        let lease = lease_of(&Scripted::new(draw_bytes(&[r as u16])));
        seen[lease.system.bits() as usize] += 1;
    }
    for (c, n) in seen.iter().enumerate() {
        let want = if (1024..6144).contains(&c) { 12 } else { 0 };
        let code = Code::from_bits(c as u16).expect("15-bit value");
        assert_eq!(*n, want, "code {code} drawn {n} times, want {want}");
    }
}

#[test]
fn detached_draw_keep_draws_only_the_writer() {
    let keep = Code::parse("3ab").unwrap();
    let src = Scripted::new(vec![0, 0, 0, 9, 0xFF, 0xFF]);
    let lease = detached_lease(&src, &ManualClock::new(T), Some(keep)).expect("detached lease");
    assert_eq!(lease.system, keep);
    assert_eq!(lease.writer, 9);
    assert_eq!(src.pos.get(), 4);
}
