//! The systems and kinds registries (spec/registry.md).

use std::collections::BTreeMap;
use std::fmt;

use crate::{Class, Code, SYS};

const SYSTEMS_HEADER: &str = "code\tname\towner\thome\tstate\tserved_by";
const KINDS_HEADER: &str = "system\tkind\trole\tstate\tserved_by\tmeaning";

/// The state of a system or a kind.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum State {
    /// Minting is allowed.
    Live,
    /// Ids stay readable; nothing mints.
    Closed,
    /// Kept for history.
    Retired,
}

/// The role of a kind; a typed field checks it.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Role {
    Entity,
    Activation,
    Command,
    Run,
    Stream,
    Store,
    Database,
    Transaction,
    Correlation,
    Request,
}

/// A row of `registry/systems.tsv`.
#[derive(Clone, Debug)]
pub struct SystemRow {
    pub code: Code,
    pub name: String,
    pub owner: String,
    pub home: String,
    pub state: State,
    pub served_by: Option<Code>,
}

/// A row of `registry/kinds.tsv`.
#[derive(Clone, Debug)]
pub struct KindRow {
    pub system: Code,
    pub kind: Code,
    pub role: Role,
    pub state: State,
    pub served_by: Option<Code>,
    pub meaning: String,
}

/// A refusal to load a registry file, with the 1-based line number in that file.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub struct RegistryError {
    pub reason: &'static str,
    pub line: usize,
}

impl fmt::Display for RegistryError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{} at line {}", self.reason, self.line)
    }
}

impl std::error::Error for RegistryError {}

/// Both registries, loaded and checked.
#[derive(Clone, Debug, Default)]
pub struct Registry {
    systems: BTreeMap<Code, SystemRow>,
    kinds: BTreeMap<(Code, Code), KindRow>,
}

fn rows(text: &str, header: &str, width: usize) -> Result<Vec<(usize, Vec<String>)>, RegistryError> {
    let mut out = Vec::new();
    let mut seen_header = false;
    for (i, line) in text.lines().enumerate() {
        let n = i + 1;
        if line.starts_with('#') {
            continue;
        }
        if !seen_header {
            if line != header {
                return Err(RegistryError { reason: "registry-header", line: n });
            }
            seen_header = true;
            continue;
        }
        let cells: Vec<String> = line.split('\t').map(str::to_owned).collect();
        if cells.len() != width || cells.iter().any(String::is_empty) {
            return Err(RegistryError { reason: "registry-row", line: n });
        }
        out.push((n, cells));
    }
    if !seen_header {
        return Err(RegistryError { reason: "registry-header", line: 1 });
    }
    Ok(out)
}

fn state(s: &str, line: usize) -> Result<State, RegistryError> {
    match s {
        "live" => Ok(State::Live),
        "closed" => Ok(State::Closed),
        "retired" => Ok(State::Retired),
        _ => Err(RegistryError { reason: "registry-state", line }),
    }
}

fn role(s: &str, line: usize) -> Result<Role, RegistryError> {
    Ok(match s {
        "entity" => Role::Entity,
        "activation" => Role::Activation,
        "command" => Role::Command,
        "run" => Role::Run,
        "stream" => Role::Stream,
        "store" => Role::Store,
        "database" => Role::Database,
        "transaction" => Role::Transaction,
        "correlation" => Role::Correlation,
        "request" => Role::Request,
        _ => return Err(RegistryError { reason: "registry-role", line }),
    })
}

fn served_by(s: &str) -> Option<Option<Code>> {
    if s == "-" { Some(None) } else { Code::parse(s).map(Some) }
}

impl Registry {
    /// Loads the text of `systems.tsv` and `kinds.tsv`. A kind file's line numbers are reported as they are.
    pub fn parse(systems: &str, kinds: &str) -> Result<Registry, RegistryError> {
        let mut reg = Registry::default();
        let mut served: Vec<(usize, Code)> = Vec::new();
        for (n, c) in rows(systems, SYSTEMS_HEADER, 6)? {
            let code = Code::parse(&c[0])
                .filter(|x| x.class() == Class::Registered)
                .ok_or(RegistryError { reason: "registry-code", line: n })?;
            let st = state(&c[4], n)?;
            let sb = served_by(&c[5]).ok_or(RegistryError { reason: "registry-served-by", line: n })?;
            if let Some(s) = sb {
                served.push((n, s));
            }
            let row = SystemRow { code, name: c[1].clone(), owner: c[2].clone(), home: c[3].clone(), state: st, served_by: sb };
            if reg.systems.insert(code, row).is_some() {
                return Err(RegistryError { reason: "registry-duplicate", line: n });
            }
        }
        for (n, s) in served {
            if !reg.systems.contains_key(&s) {
                return Err(RegistryError { reason: "registry-served-by", line: n });
            }
        }
        for (n, c) in rows(kinds, KINDS_HEADER, 6)? {
            let system = Code::parse(&c[0]).ok_or(RegistryError { reason: "registry-code", line: n })?;
            if !reg.systems.contains_key(&system) {
                return Err(RegistryError { reason: "registry-system", line: n });
            }
            let kind = Code::parse(&c[1])
                .filter(|k| k.is_kind() && *k != SYS)
                .ok_or(RegistryError { reason: "registry-code", line: n })?;
            let rl = role(&c[2], n)?;
            let st = state(&c[3], n)?;
            let sb = served_by(&c[4]).ok_or(RegistryError { reason: "registry-served-by", line: n })?;
            if let Some(s) = sb
                && !reg.systems.contains_key(&s)
            {
                return Err(RegistryError { reason: "registry-served-by", line: n });
            }
            let row = KindRow { system, kind, role: rl, state: st, served_by: sb, meaning: c[5].clone() };
            if reg.kinds.insert((system, kind), row).is_some() {
                return Err(RegistryError { reason: "registry-duplicate", line: n });
            }
        }
        Ok(reg)
    }

    /// The row of a system.
    pub fn system(&self, code: Code) -> Option<&SystemRow> {
        self.systems.get(&code)
    }

    /// The row of a kind of a system.
    pub fn kind(&self, system: Code, kind: Code) -> Option<&KindRow> {
        self.kinds.get(&(system, kind))
    }

    /// All systems, in code order.
    pub fn systems(&self) -> impl Iterator<Item = &SystemRow> {
        self.systems.values()
    }

    /// The kinds of one system, in code order.
    pub fn kinds_of(&self, system: Code) -> impl Iterator<Item = &KindRow> {
        self.kinds.range((system, Code::from_bits(0).unwrap_or(system))..).take_while(move |(k, _)| k.0 == system).map(|(_, r)| r)
    }
}
