//! Helpers shared by the integration tests: the vectors, hex, a seeded generator and the registries.

#![allow(dead_code)]

use std::path::PathBuf;

use eid::Registry;

pub fn root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..").join("..")
}

/// The data rows of a vector file: comment lines dropped, cells split on tabs.
pub fn tsv(name: &str) -> Vec<Vec<String>> {
    let path = root().join("vectors").join(name);
    let text = std::fs::read_to_string(&path).unwrap_or_else(|e| panic!("read {}: {e}", path.display()));
    text.lines()
        .filter(|l| !l.is_empty() && !l.starts_with('#'))
        .map(|l| l.split('\t').map(str::to_owned).collect())
        .collect()
}

pub fn unhex(s: &str) -> Vec<u8> {
    if s == "-" {
        return Vec::new();
    }
    (0..s.len()).step_by(2).map(|i| u8::from_str_radix(&s[i..i + 2], 16).expect("hex")).collect()
}

pub fn hex(b: &[u8]) -> String {
    b.iter().map(|x| format!("{x:02x}")).collect()
}

/// splitmix64: a seeded generator for reproducible random ids.
pub struct Mix(pub u64);

impl Mix {
    pub fn next(&mut self) -> u64 {
        self.0 = self.0.wrapping_add(0x9E37_79B9_7F4A_7C15);
        let mut z = self.0;
        z = (z ^ (z >> 30)).wrapping_mul(0xBF58_476D_1CE4_E5B9);
        z = (z ^ (z >> 27)).wrapping_mul(0x94D0_49BB_1331_11EB);
        z ^ (z >> 31)
    }

    pub fn below(&mut self, n: u64) -> u64 {
        self.next() % n
    }
}

/// The registries in the repository.
pub fn repo_registry() -> Registry {
    let s = std::fs::read_to_string(root().join("registry").join("systems.tsv")).expect("systems.tsv");
    let k = std::fs::read_to_string(root().join("registry").join("kinds.tsv")).expect("kinds.tsv");
    Registry::parse(&s, &k).expect("registry loads")
}

/// A small registry whose systems have homes, for allocator tests.
pub fn homed_registry() -> Registry {
    Registry::parse(
        "code\tname\towner\thome\tstate\tserved_by\n\
         emq\tEchoMQ\t-\th1\tlive\t-\n\
         kan\tKAN\t-\th1\tlive\t-\n\
         brd\tretired\t-\t-\tretired\t-\n",
        "system\tkind\trole\tstate\tserved_by\tmeaning\n\
         emq\tevt\tstream\tlive\t-\tevent\n\
         emq\tjob\tcommand\tlive\t-\tjob\n\
         kan\tdoc\tentity\tlive\t-\tdocument\n",
    )
    .expect("homed registry")
}
