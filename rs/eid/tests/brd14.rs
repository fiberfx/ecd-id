//! G14: the brd14 embedding against vectors/brd14.tsv, both ways, pure and through the registry layer; order and the
//! inverse over random brd14 ids.

mod common;

use std::collections::{HashMap, HashSet};

use common::{Mix, repo_registry, tsv, unhex};
use eid::{BRD14_EPOCH, Id, Refusal, from_brd14, to_brd14};

const BASE62: &[u8; 62] = b"0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";

fn answer<T: ToString>(r: Result<T, Refusal>) -> String {
    match r {
        Ok(v) => v.to_string(),
        Err(e) => format!("!{}", e.reason()),
    }
}

#[test]
fn g14_brd14_vectors_both_ways() {
    let reg = repo_registry();
    let rows = tsv("brd14.tsv");
    // (case, op) that a registry row refuses
    let refused: HashSet<(String, String)> =
        rows.iter().filter(|r| r[2] == "registry").map(|r| (r[0].clone(), r[1].clone())).collect();
    // (case, op) whose pure row accepts
    let mut accepted: HashSet<(String, String)> = HashSet::new();
    let mut seen = HashSet::new();
    let mut n: HashMap<String, usize> = HashMap::new();
    for r in &rows {
        let key = (r[0].clone(), r[1].clone());
        let (got, got_reg) = match r[1].as_str() {
            "from" => {
                let b = unhex(&r[3]);
                (answer(from_brd14(&b)), answer(reg.from_brd14(&b)))
            }
            "to" => {
                let id = Id::parse(r[3].as_bytes()).unwrap_or_else(|e| panic!("case {}: {e}", r[0]));
                (answer(to_brd14(id)), answer(reg.to_brd14(id)))
            }
            op => panic!("case {}: op {op}", r[0]),
        };
        let want = r[4].as_str();
        match r[2].as_str() {
            "pure" => {
                assert_eq!(got, want, "case {} {} {}: pure", r[0], r[1], r[3]);
                if !refused.contains(&key) {
                    assert_eq!(got_reg, want, "case {} {} {}: registry layer", r[0], r[1], r[3]);
                }
                if !want.starts_with('!') {
                    accepted.insert(key);
                }
            }
            "registry" => {
                assert_eq!(got_reg, want, "case {} {} {}: registry layer", r[0], r[1], r[3]);
                assert_eq!(want, "!brd14-kind", "case {} {}: the registry layer adds brd14-kind only", r[0], r[1]);
                assert!(accepted.contains(&key), "case {} {}: no accepting pure row before it", r[0], r[1]);
            }
            layer => panic!("case {}: layer {layer}", r[0]),
        }
        if let Some(x) = want.strip_prefix('!') {
            seen.insert(x.to_owned());
        }
        *n.entry(r[2].clone()).or_default() += 1;
    }
    for reason in [
        "brd14-length",
        "brd14-namespace",
        "brd14-alphabet",
        "brd14-range",
        "brd14-system",
        "brd14-time",
        "brd14-writer",
        "brd14-count",
        "brd14-kind",
    ] {
        assert!(seen.contains(reason), "no vector for {reason}");
    }
    println!("G14 brd14: {} pure and {} registry rows agree, both ways; {} reasons", n["pure"], n["registry"], seen.len());
}

#[test]
fn g14_brd14_order_and_inverse() {
    let pairs: Vec<(String, Id)> = tsv("brd14.tsv")
        .into_iter()
        .filter(|r| r[1] == "to" && r[2] == "pure" && !r[4].starts_with('!'))
        .map(|r| (r[4].clone(), Id::parse(r[3].as_bytes()).expect("a valid id")))
        .collect();
    assert!(pairs.len() > 50);
    for (a, x) in &pairs {
        for (b, y) in &pairs {
            assert_eq!(a.cmp(b), x.cmp(y), "order of {a} and {b}");
        }
    }
    let mut m = Mix(14);
    let mut prev: Option<(Vec<u8>, Id)> = None;
    let n = 100_000;
    for _ in 0..n {
        let mut t = vec![0u8; 14];
        for c in t.iter_mut().take(3) {
            *c = b'A' + m.below(26) as u8;
        }
        let mut s = m.next() >> 1;
        let (ts, node, seq) = (s >> 22, (s >> 12) & 1023, s & 4095);
        for c in t[3..].iter_mut().rev() {
            *c = BASE62[(s % 62) as usize];
            s /= 62;
        }
        let id = from_brd14(&t).unwrap_or_else(|e| panic!("{}: {e}", String::from_utf8_lossy(&t)));
        assert_eq!((id.time(), id.writer() as u64, id.count() as u64), (BRD14_EPOCH + ts, node, seq));
        assert_eq!(to_brd14(id).expect("returns").as_bytes(), &t[..]);
        if let Some((pt, pid)) = &prev {
            assert_eq!(pt.cmp(&t), pid.cmp(&id), "order of {} and {}", String::from_utf8_lossy(pt), id);
        }
        prev = Some((t, id));
    }
    println!("G14 brd14: {} vector ids keep brd14 order pairwise; {n} random ids return and keep order", pairs.len());
}
