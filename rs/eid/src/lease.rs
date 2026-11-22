//! Leases, writer numbers and the allocator (spec/lease.md).

use std::collections::{HashMap, HashSet};
use std::fmt;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex, MutexGuard};
use std::time::{SystemTime, UNIX_EPOCH};

use crate::{Class, Code, Refusal, Registry, State, TIME_MAX, WRITER_MAX};

/// The longest lease window: 24 hours in milliseconds.
pub const LEASE_TERM_MAX: u64 = 86_400_000;
/// A writer number's second comes round after this many seconds (36.4 hours).
pub const SECOND_SLOTS: u64 = 1 << 17;
/// Grants per second per system.
pub const INDEX_SLOTS: u32 = 1 << 13;

/// A source of Unix milliseconds.
pub trait Clock: Send + Sync {
    /// The current Unix time in milliseconds.
    fn now_ms(&self) -> u64;
}

/// The host's wall clock.
#[derive(Clone, Copy, Debug, Default)]
pub struct SystemClock;

impl Clock for SystemClock {
    fn now_ms(&self) -> u64 {
        SystemTime::now().duration_since(UNIX_EPOCH).map(|d| d.as_millis() as u64).unwrap_or(0)
    }
}

/// A clock that moves only when told to; for tests of restarts, clocks set back and bursts.
#[derive(Debug, Default)]
pub struct ManualClock(AtomicU64);

impl ManualClock {
    /// A clock reading `ms`.
    pub fn new(ms: u64) -> ManualClock {
        ManualClock(AtomicU64::new(ms))
    }

    /// Sets the reading.
    pub fn set(&self, ms: u64) {
        self.0.store(ms, Ordering::SeqCst);
    }

    /// Moves the reading forward by `ms`.
    pub fn advance(&self, ms: u64) {
        self.0.fetch_add(ms, Ordering::SeqCst);
    }
}

impl Clock for ManualClock {
    fn now_ms(&self) -> u64 {
        self.0.load(Ordering::SeqCst)
    }
}

/// An exclusive right to one writer number and the id-times from `not_before` to `not_after`, inclusive.
#[derive(Clone, Copy, PartialEq, Eq, Hash, Debug)]
pub struct Lease {
    /// The system that granted the lease.
    pub system: Code,
    /// The writer number.
    pub writer: u32,
    /// The first id-time the lease covers, in Unix milliseconds; at least 1.
    pub not_before: u64,
    /// The last id-time the lease covers.
    pub not_after: u64,
}

impl Lease {
    /// A lease, checked as the text form is checked.
    pub fn new(system: Code, writer: u32, not_before: u64, not_after: u64) -> Result<Lease, Refusal> {
        if system.class() == Class::Reserved {
            return Err(Refusal::LeaseSystem);
        }
        if writer > WRITER_MAX {
            return Err(Refusal::LeaseWriter);
        }
        if not_before == 0 || not_after < not_before || not_after > TIME_MAX || not_after - not_before > LEASE_TERM_MAX
        {
            return Err(Refusal::LeaseWindow);
        }
        Ok(Lease { system, writer, not_before, not_after })
    }

    /// Parses `system:writer:not_before:not_after`.
    pub fn parse(s: &str) -> Result<Lease, Refusal> {
        let parts: Vec<&str> = s.split(':').collect();
        if parts.len() != 4 {
            return Err(Refusal::LeaseSyntax);
        }
        let system = Code::parse(parts[0]).ok_or(Refusal::LeaseSyntax)?;
        let writer = decimal(parts[1])?;
        let not_before = decimal(parts[2])?;
        let not_after = decimal(parts[3])?;
        if system.class() == Class::Reserved {
            return Err(Refusal::LeaseSystem);
        }
        if writer > WRITER_MAX as u64 {
            return Err(Refusal::LeaseWriter);
        }
        Lease::new(system, writer as u32, not_before, not_after)
    }

    /// The lease passed in the environment variable `EID_LEASE`, if it is set.
    pub fn from_env() -> Option<Result<Lease, Refusal>> {
        std::env::var("EID_LEASE").ok().map(|v| Lease::parse(&v))
    }
}

fn decimal(p: &str) -> Result<u64, Refusal> {
    let b = p.as_bytes();
    if b.is_empty() || b.len() > 20 || !b.iter().all(u8::is_ascii_digit) || (b.len() > 1 && b[0] == b'0') {
        return Err(Refusal::LeaseSyntax);
    }
    p.parse::<u64>().map_err(|_| Refusal::LeaseSyntax)
}

impl fmt::Display for Lease {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}:{}:{}:{}", self.system, self.writer, self.not_before, self.not_after)
    }
}

/// `writer = (second mod 2^17) × 2^13 + index`.
pub fn writer_for(second: u64, index: u32) -> u32 {
    (((second % SECOND_SLOTS) as u32) << 13) | (index % INDEX_SLOTS)
}

/// The allocator's durable state in a system's own store: one number per system, and the live activation.
pub trait LeaseStore: Send + Sync {
    /// The stored grant second of `system`, if any.
    fn last_second(&self, system: Code) -> Option<u64>;
    /// Sets the stored grant second to `new` if it equals `expected`; returns whether it did.
    fn compare_and_set(&self, system: Code, expected: Option<u64>, new: u64) -> bool;
    /// Records a live activation of `system`; returns false if one is already live.
    fn activate(&self, system: Code) -> bool;
    /// Ends the live activation of `system`; called by the supervisor when the allocator's process ends.
    fn release(&self, system: Code);
}

#[derive(Clone, Debug, Default)]
struct MemState {
    last: HashMap<Code, u64>,
    active: HashSet<Code>,
}

/// A lease store in memory. `snapshot` copies it, which stands for a backup that is later restored or cloned.
#[derive(Debug, Default)]
pub struct MemoryStore(Mutex<MemState>);

impl MemoryStore {
    /// An empty store.
    pub fn new() -> MemoryStore {
        MemoryStore::default()
    }

    /// An independent copy of the store as it is now.
    pub fn snapshot(&self) -> MemoryStore {
        MemoryStore(Mutex::new(self.lock().clone()))
    }

    fn lock(&self) -> MutexGuard<'_, MemState> {
        self.0.lock().unwrap_or_else(|e| e.into_inner())
    }
}

impl LeaseStore for MemoryStore {
    fn last_second(&self, system: Code) -> Option<u64> {
        self.lock().last.get(&system).copied()
    }

    fn compare_and_set(&self, system: Code, expected: Option<u64>, new: u64) -> bool {
        let mut st = self.lock();
        if st.last.get(&system).copied() != expected {
            return false;
        }
        st.last.insert(system, new);
        true
    }

    fn activate(&self, system: Code) -> bool {
        self.lock().active.insert(system)
    }

    fn release(&self, system: Code) {
        self.lock().active.remove(&system);
    }
}

/// The allocator of one system: grants leases from the clock second and one stored number (spec/lease.md#allocator).
pub struct Allocator {
    system: Code,
    store: Arc<dyn LeaseStore>,
    clock: Arc<dyn Clock>,
    offset: u32,
    run_second: Option<u64>,
    granted: u32,
    #[cfg(test)]
    pub(crate) skip_start_wait: bool,
}

impl Allocator {
    /// Starts the allocator of `system` under the supervisor `home`. `offset` is the random 13-bit index offset,
    /// drawn once per start (see `OsRandom`).
    pub fn start(
        registry: &Registry,
        system: Code,
        home: &str,
        store: Arc<dyn LeaseStore>,
        clock: Arc<dyn Clock>,
        offset: u32,
    ) -> Result<Allocator, Refusal> {
        if system.class() != Class::Registered {
            return Err(Refusal::Class);
        }
        let row = registry.system(system).ok_or(Refusal::Unknown)?;
        if row.state != State::Live {
            return Err(Refusal::State);
        }
        if row.home == "-" || row.home != home {
            return Err(Refusal::NotHome);
        }
        if !store.activate(system) {
            return Err(Refusal::Active);
        }
        Ok(Allocator {
            system,
            store,
            clock,
            offset: offset % INDEX_SLOTS,
            run_second: None,
            granted: 0,
            #[cfg(test)]
            skip_start_wait: false,
        })
    }

    /// The system this allocator serves.
    pub fn system(&self) -> Code {
        self.system
    }

    /// Grants a lease of `term_ms` milliseconds (at most 24 hours) from the current clock second.
    pub fn grant(&mut self, term_ms: u64) -> Result<Lease, Refusal> {
        if term_ms == 0 || term_ms > LEASE_TERM_MAX {
            return Err(Refusal::Term);
        }
        let s = self.clock.now_ms() / 1000;
        if s == 0 {
            return Err(Refusal::ClockZero);
        }
        let last = self.store.last_second(self.system);
        match self.run_second {
            None => {
                if let Some(l) = last {
                    if s < l {
                        return Err(Refusal::ClockBehind);
                    }
                    if s == l && !self.start_wait_skipped() {
                        return Err(Refusal::Wait);
                    }
                }
                if last != Some(s) && !self.store.compare_and_set(self.system, last, s) {
                    return Err(Refusal::Contended);
                }
                self.run_second = Some(s);
                self.granted = 0;
            }
            Some(rs) => {
                if last != Some(rs) {
                    return Err(Refusal::Contended);
                }
                if s < rs {
                    return Err(Refusal::ClockBehind);
                }
                if s == rs {
                    if self.granted + 1 >= INDEX_SLOTS {
                        return Err(Refusal::SecondFull);
                    }
                    self.granted += 1;
                } else {
                    if !self.store.compare_and_set(self.system, Some(rs), s) {
                        return Err(Refusal::Contended);
                    }
                    self.run_second = Some(s);
                    self.granted = 0;
                }
            }
        }
        let not_before = s * 1000;
        let not_after = (not_before + term_ms).min(TIME_MAX);
        Lease::new(self.system, writer_for(s, self.offset + self.granted), not_before, not_after)
    }

    #[cfg(test)]
    fn start_wait_skipped(&self) -> bool {
        self.skip_start_wait
    }

    #[cfg(not(test))]
    fn start_wait_skipped(&self) -> bool {
        false
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const T0: u64 = 1_795_271_400_000;
    const TERM: u64 = LEASE_TERM_MAX;

    fn registry() -> Registry {
        Registry::parse(
            "code\tname\towner\thome\tstate\tserved_by\nemq\tEchoMQ\t-\th1\tlive\t-\n",
            "system\tkind\trole\tstate\tserved_by\tmeaning\nemq\tevt\tstream\tlive\t-\tevent\n",
        )
        .expect("registry")
    }

    fn overlapping(leases: &[Lease]) -> usize {
        let mut by_writer: HashMap<u32, Vec<(u64, u64)>> = HashMap::new();
        for l in leases {
            by_writer.entry(l.writer).or_default().push((l.not_before, l.not_after));
        }
        let mut repeats = 0;
        for windows in by_writer.values_mut() {
            windows.sort();
            for w in windows.windows(2) {
                if w[1].0 <= w[0].1 {
                    repeats += 1;
                }
            }
        }
        repeats
    }

    /// Runs `restarts` kill-and-restart cycles; each run grants one to three leases. The offset is fixed, so only
    /// the allocator's rules keep writers apart.
    fn restart_cycles(restarts: usize, skip_wait: bool) -> (Vec<Lease>, usize) {
        let reg = registry();
        let emq = Code::parse("emq").unwrap();
        let store: Arc<MemoryStore> = Arc::new(MemoryStore::new());
        let clock = Arc::new(ManualClock::new(T0));
        let mut leases = Vec::new();
        let mut same_second_restarts = 0;
        let mut x: u64 = 0x9E37_79B9_7F4A_7C15;
        for _ in 0..restarts {
            let mut a = Allocator::start(&reg, emq, "h1", store.clone(), clock.clone(), 0).expect("start");
            a.skip_start_wait = skip_wait;
            x ^= x << 13;
            x ^= x >> 7;
            x ^= x << 17;
            let grants = 1 + (x % 3) as usize;
            for _ in 0..grants {
                loop {
                    match a.grant(TERM) {
                        Ok(l) => {
                            leases.push(l);
                            break;
                        }
                        Err(Refusal::Wait) => {
                            same_second_restarts += 1;
                            let now = clock.now_ms();
                            clock.set((now / 1000 + 1) * 1000 + now % 7);
                        }
                        Err(e) => panic!("grant refused: {e}"),
                    }
                }
            }
            // The process is killed; its supervisor ends the activation. The next start comes 0 to 699 ms later.
            store.release(emq);
            clock.advance(x % 700);
        }
        (leases, same_second_restarts)
    }

    #[test]
    fn g5_restart_never_reissues_a_live_writer() {
        let (leases, same_second) = restart_cycles(1000, false);
        let repeats = overlapping(&leases);
        println!("G5 good: {} leases over 1000 restarts, {} restarts inside one second, repeats {}", leases.len(), same_second, repeats);
        assert!(leases.len() >= 1000);
        assert!(same_second > 0, "no restart fell inside one second");
        assert_eq!(repeats, 0);
    }

    #[test]
    fn g5_control_without_the_wait_a_writer_repeats() {
        let (leases, _) = restart_cycles(1000, true);
        let repeats = overlapping(&leases);
        println!("G5 control (wait removed): {} leases, repeats {}", leases.len(), repeats);
        assert!(repeats > 0, "removing the wait should reissue a live writer");
    }

    #[test]
    fn g5_grants_inside_one_run_and_second_are_distinct() {
        let reg = registry();
        let emq = Code::parse("emq").unwrap();
        let store: Arc<MemoryStore> = Arc::new(MemoryStore::new());
        let clock = Arc::new(ManualClock::new(T0 + 500));
        let mut a = Allocator::start(&reg, emq, "h1", store.clone(), clock.clone(), 8190).unwrap();
        let mut writers = HashSet::new();
        for _ in 0..INDEX_SLOTS {
            assert!(writers.insert(a.grant(TERM).unwrap().writer));
        }
        assert_eq!(a.grant(TERM), Err(Refusal::SecondFull));
        clock.advance(1000);
        assert!(writers.insert(a.grant(TERM).unwrap().writer));
        assert_eq!(store.last_second(emq), Some(T0 / 1000 + 1));
    }
}
