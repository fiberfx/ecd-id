//! A registered system's door: what it admits as its own, what it binds as a command token, and what it refuses
//! (spec/lease.md#door).

use std::collections::HashMap;
use std::sync::Arc;

use crate::{Class, Clock, Code, Id, Lease, Minter, Refusal};

/// The result of an admission.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Admission {
    /// The id is the system's own and is stored as such.
    Own(Id),
    /// The id was detached: the door minted `id` and bound `token` to it. `token` is never stored as a fact.
    Bound { token: Id, id: Id },
}

/// The door of one registered system.
pub struct Door {
    minter: Minter,
    kinds: Vec<Code>,
    ahead_ms: u64,
    clock: Arc<dyn Clock>,
    tokens: HashMap<Id, Id>,
}

impl Door {
    /// A door that mints with `minter` (its lease names the door's system), accepts `kinds`, and refuses ids
    /// stamped more than `ahead_ms` milliseconds ahead of `clock`.
    pub fn new(minter: Minter, kinds: &[Code], ahead_ms: u64, clock: Arc<dyn Clock>) -> Door {
        Door { minter, kinds: kinds.to_vec(), ahead_ms, clock, tokens: HashMap::new() }
    }

    /// The door's system.
    pub fn system(&self) -> Code {
        self.minter.lease().system
    }

    /// Admits `id`. `lease` is the lease this system issued to the sending session, when it issued one.
    pub fn admit(&mut self, id: Id, lease: Option<&Lease>) -> Result<Admission, Refusal> {
        if !self.kinds.contains(&id.kind()) {
            return Err(Refusal::Kind);
        }
        if id.class() == Class::Detached {
            if let Some(&own) = self.tokens.get(&id) {
                return Ok(Admission::Bound { token: id, id: own });
            }
            let own = self.minter.mint(id.kind())?;
            self.tokens.insert(id, own);
            return Ok(Admission::Bound { token: id, id: own });
        }
        if id.system() != self.system() {
            return Err(Refusal::Foreign);
        }
        if id.time() > self.clock.now_ms().saturating_add(self.ahead_ms) {
            return Err(Refusal::Ahead);
        }
        if let Some(l) = lease
            && (l.system != id.system() || l.writer != id.writer() || id.time() < l.not_before || id.time() > l.not_after)
        {
            return Err(Refusal::Lease);
        }
        Ok(Admission::Own(id))
    }

    /// The number of detached ids bound as command tokens.
    pub fn bound(&self) -> usize {
        self.tokens.len()
    }
}
