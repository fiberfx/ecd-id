//! The minter, its pid guard and detached minting (spec/lease.md#minter).

use std::fs::File;
use std::io::Read;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};

use crate::{COUNT_LIMIT, Class, Clock, Code, Id, LEASE_TERM_MAX, Lease, Refusal, TIME_MAX, WRITER_MAX};

/// A source of the current process id.
pub trait PidSource: Send + Sync {
    /// The id of the process that calls.
    fn pid(&self) -> u32;
}

/// The operating system's process id.
#[derive(Clone, Copy, Debug, Default)]
pub struct ProcessPid;

impl PidSource for ProcessPid {
    fn pid(&self) -> u32 {
        std::process::id()
    }
}

/// A process id that changes only when told to; for tests of the pid guard.
#[derive(Debug, Default)]
pub struct ManualPid(AtomicU32);

impl ManualPid {
    /// A source reporting `pid`.
    pub fn new(pid: u32) -> ManualPid {
        ManualPid(AtomicU32::new(pid))
    }

    /// Changes the reported pid.
    pub fn set(&self, pid: u32) {
        self.0.store(pid, Ordering::SeqCst);
    }
}

impl PidSource for ManualPid {
    fn pid(&self) -> u32 {
        self.0.load(Ordering::SeqCst)
    }
}

/// Mints ids inside one lease with no round trip. One lease feeds one minter; the minter is not shared between
/// threads without a lock.
#[derive(Clone)]
pub struct Minter {
    lease: Lease,
    last_t: Option<u64>,
    count: u32,
    clock: Arc<dyn Clock>,
    pids: Arc<dyn PidSource>,
    pid: u32,
}

impl Minter {
    /// A minter for `lease`, guarded by the process id `pids` reports now.
    pub fn new(lease: Lease, clock: Arc<dyn Clock>, pids: Arc<dyn PidSource>) -> Minter {
        let pid = pids.pid();
        Minter { lease, last_t: None, count: 0, clock, pids, pid }
    }

    /// The lease in force, with `not_before` as raised by the last switch.
    pub fn lease(&self) -> Lease {
        self.lease
    }

    /// Mints one id of `kind`.
    pub fn mint(&mut self, kind: Code) -> Result<Id, Refusal> {
        if self.pids.pid() != self.pid {
            return Err(Refusal::Fork);
        }
        if !kind.is_kind() {
            return Err(Refusal::Kind);
        }
        let mut t = self.clock.now_ms().max(self.lease.not_before);
        if let Some(last) = self.last_t {
            t = t.max(last);
        }
        let mut count = if Some(t) == self.last_t { self.count + 1 } else { 0 };
        if count == COUNT_LIMIT {
            t += 1;
            count = 0;
        }
        if t > self.lease.not_after {
            return Err(Refusal::LeaseEnded);
        }
        self.last_t = Some(t);
        self.count = count;
        Ok(Id::pack(self.lease.system, kind, t, self.lease.writer, count))
    }

    /// Mints one id of the kind named by `kind`.
    pub fn mint_kind(&mut self, kind: &str) -> Result<Id, Refusal> {
        let k = Code::parse(kind).ok_or(Refusal::Kind)?;
        self.mint(k)
    }

    /// Switches to a fresh lease of the same system, raising its `not_before` past the last id minted.
    pub fn switch(&mut self, lease: Lease) -> Result<(), Refusal> {
        if self.pids.pid() != self.pid {
            return Err(Refusal::Fork);
        }
        if lease.system != self.lease.system {
            return Err(Refusal::LeaseSystem);
        }
        let mut next = lease;
        if let Some(last) = self.last_t {
            next.not_before = next.not_before.max(last + 1);
        }
        if next.not_before > next.not_after {
            return Err(Refusal::LeaseEnded);
        }
        self.lease = next;
        Ok(())
    }
}

/// A source of random bytes.
pub trait RandomSource {
    /// Fills `buf`, or fails; a failure is never replaced by a weaker source.
    fn fill(&self, buf: &mut [u8]) -> Result<(), Refusal>;
}

/// The operating system's random source, read from `/dev/urandom`.
#[derive(Clone, Copy, Debug, Default)]
pub struct OsRandom;

impl RandomSource for OsRandom {
    fn fill(&self, buf: &mut [u8]) -> Result<(), Refusal> {
        File::open("/dev/urandom").and_then(|mut f| f.read_exact(buf)).map_err(|_| Refusal::Random)
    }
}

impl OsRandom {
    /// A random 13-bit index offset for an allocator start.
    pub fn offset(&self) -> Result<u32, Refusal> {
        let mut b = [0u8; 2];
        self.fill(&mut b)?;
        Ok(u16::from_be_bytes(b) as u32 & 0x1fff)
    }
}

/// A detached lease: a detached system code (kept if `keep` is given, drawn otherwise), a random writer, and a
/// window of 24 hours from the clock.
pub fn detached_lease(random: &dyn RandomSource, clock: &dyn Clock, keep: Option<Code>) -> Result<Lease, Refusal> {
    let system = match keep {
        Some(c) if c.class() == Class::Detached => c,
        Some(_) => return Err(Refusal::LeaseSystem),
        None => draw_detached_code(random)?,
    };
    let mut w = [0u8; 4];
    random.fill(&mut w)?;
    let writer = u32::from_be_bytes(w) & WRITER_MAX;
    let not_before = clock.now_ms().clamp(1, TIME_MAX - LEASE_TERM_MAX);
    Lease::new(system, writer, not_before, not_before + LEASE_TERM_MAX)
}

fn draw_detached_code(random: &dyn RandomSource) -> Result<Code, Refusal> {
    // Rejection sampling keeps the five first characters equally likely.
    for _ in 0..64 {
        let mut b = [0u8; 2];
        random.fill(&mut b)?;
        if let Some(c) = Code::from_bits(u16::from_be_bytes(b) & 0x7fff)
            && c.class() == Class::Detached
        {
            return Ok(c);
        }
    }
    Err(Refusal::Random)
}
