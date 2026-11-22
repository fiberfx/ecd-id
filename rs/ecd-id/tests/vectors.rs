//! Conformance against vectors/: the text and the bytes both ways, every refusal and its reason, leases, minting,
//! stream ids, the brd14 embedding and mxdb's identities.

mod common;

use std::collections::HashSet;
use std::sync::Arc;

use common::{hex, repo_registry, tsv, unhex};
use ecd_id::{
    ALPHABET, Class, Code, Id, Lease, ManualClock, ManualPid, Minter, Refusal, StreamId, embed, from_stream, unembed,
    writer_for,
};

fn class_name(c: Class) -> &'static str {
    match c {
        Class::Registered => "registered",
        Class::Detached => "detached",
        Class::Reserved => "reserved",
    }
}

#[test]
fn g1_valid_both_ways() {
    let rows = tsv("valid.tsv");
    assert!(rows.len() > 200);
    for r in &rows {
        let id = Id::parse(r[0].as_bytes()).unwrap_or_else(|e| panic!("{}: {e}", r[0]));
        assert_eq!(hex(&id.to_bytes()), r[1], "bytes of {}", r[0]);
        let back = Id::from_bytes(&unhex(&r[1])).expect("binary ingress");
        assert_eq!(back, id);
        assert_eq!(id.to_string(), r[0]);
        assert_eq!(id.system().to_string(), r[2]);
        assert_eq!(id.kind().to_string(), r[3]);
        assert_eq!(id.time().to_string(), r[4]);
        assert_eq!(id.writer().to_string(), r[5]);
        assert_eq!(id.count().to_string(), r[6]);
        assert_eq!(class_name(id.class()), r[7]);
        assert_eq!(id.stream_id().to_string(), r[8]);
        let rebuilt = Id::from_parts(id.system(), id.kind(), id.time(), id.writer(), id.count()).expect("from parts");
        assert_eq!(rebuilt, id);
    }
    println!("G1 valid: {} ids agree both ways", rows.len());
}

#[test]
fn g1_invalid_texts_refused_with_reason() {
    let rows = tsv("invalid.tsv");
    let mut seen = HashSet::new();
    for r in &rows {
        let input = unhex(&r[1]);
        match Id::parse(&input) {
            Ok(id) => panic!("{} accepted as {id}", r[0]),
            Err(e) => assert_eq!(e.reason(), r[2], "case {}", r[0]),
        }
        seen.insert(r[2].clone());
    }
    for reason in ["length", "separator", "alphabet", "pad", "reserved", "system-reserved", "kind"] {
        assert!(seen.contains(reason), "no vector for {reason}");
    }
    let cases: HashSet<&str> = rows.iter().map(|r| r[0].as_str()).collect();
    for c in ["embedded-cr", "embedded-lf", "non-ascii-28", "non-ascii-29", "upper", "zero", "one"] {
        assert!(cases.contains(c), "no vector {c}");
    }
    println!("G1 invalid: {} texts refused with their reasons", rows.len());
}

#[test]
fn g1_binary_invalid_refused_with_reason() {
    let rows = tsv("binary_invalid.tsv");
    for r in &rows {
        match Id::from_bytes(&unhex(&r[1])) {
            Ok(id) => panic!("{} accepted as {id}", r[0]),
            Err(e) => assert_eq!(e.reason(), r[2], "case {}", r[0]),
        }
    }
    println!("G1 binary invalid: {} inputs refused", rows.len());
}

#[test]
fn g1_lease_vectors() {
    let mut n = 0;
    for r in tsv("lease.tsv") {
        if r[0] == "writer" {
            let second: u64 = r[1].parse().unwrap();
            let index: u32 = r[2].parse().unwrap();
            assert_eq!(writer_for(second, index).to_string(), r[3], "writer for {second} {index}");
        } else {
            match Lease::parse(&r[1]) {
                Ok(l) => {
                    assert_eq!(r[2], "ok", "case {} accepted", r[0]);
                    assert_eq!(l.to_string(), r[1]);
                }
                Err(e) => assert_eq!(format!("!{}", e.reason()), r[2], "case {}", r[0]),
            }
        }
        n += 1;
    }
    println!("G1 lease: {n} rows");
}

#[test]
fn g1_mint_vectors() {
    let clock = Arc::new(ManualClock::new(0));
    let pid = Arc::new(ManualPid::new(1));
    let mut minter: Option<Minter> = None;
    let mut n = 0;
    for r in tsv("mint.tsv") {
        let (scenario, op, arg, expect) = (&r[0], r[1].as_str(), &r[3], &r[4]);
        let result = match op {
            "lease" => {
                minter = Some(Minter::new(Lease::parse(arg).expect("lease"), clock.clone(), pid.clone()));
                "ok".to_owned()
            }
            "mint" => {
                clock.set(r[2].parse().unwrap());
                match minter.as_mut().unwrap().mint_kind(arg) {
                    Ok(id) => id.to_string(),
                    Err(e) => format!("!{}", e.reason()),
                }
            }
            "burst" => {
                clock.set(r[2].parse().unwrap());
                let (kind, count) = arg.split_once('*').unwrap();
                let count: usize = count.parse().unwrap();
                let m = minter.as_mut().unwrap();
                let mut last = String::new();
                for _ in 0..count {
                    last = match m.mint_kind(kind) {
                        Ok(id) => id.to_string(),
                        Err(e) => format!("!{}", e.reason()),
                    };
                }
                last
            }
            "switch" => match minter.as_mut().unwrap().switch(Lease::parse(arg).expect("lease")) {
                Ok(()) => "ok".to_owned(),
                Err(e) => format!("!{}", e.reason()),
            },
            other => panic!("unknown op {other}"),
        };
        assert_eq!(&result, expect, "scenario {scenario}, op {op} {arg}");
        n += 1;
    }
    println!("G1 mint: {n} steps");
}

#[test]
fn g4_stream_vectors_and_inverse() {
    let rows = tsv("stream.tsv");
    for r in &rows {
        let id = Id::parse(r[0].as_bytes()).unwrap();
        let sid = id.stream_id();
        assert_eq!(sid.to_string(), r[2]);
        assert_eq!(id.legacy_tail().to_string(), r[3]);
        assert!(sid.seq < 1 << 63);
        let parsed = StreamId::parse(&r[2]).unwrap();
        assert_eq!(from_stream(&r[1], parsed).unwrap(), id, "inverse of {}", r[0]);
    }
    // The first four rows: four systems, one writer, one millisecond, count 0.
    let sids: HashSet<String> = rows[..4].iter().map(|r| r[2].clone()).collect();
    let tails: HashSet<String> = rows[..4].iter().map(|r| r[3].clone()).collect();
    assert_eq!((sids.len(), tails.len()), (4, 1));
    println!("G4 stream vectors: {} ids map and return; 4 systems on one writer give 4 stream ids, 1 legacy tail", rows.len());
}

#[test]
fn g4_stream_invalid_refused() {
    let rows = tsv("stream_invalid.tsv");
    for r in &rows {
        let text = if r[2] == "-" { "" } else { r[2].as_str() };
        let got = StreamId::parse(text).and_then(|sid| from_stream(&r[1], sid));
        match got {
            Ok(id) => panic!("{} accepted as {id}", r[0]),
            Err(e) => assert_eq!(e.reason(), r[3], "case {}", r[0]),
        }
    }
    println!("G4 stream invalid: {} refused", rows.len());
}

#[test]
fn g8_brd14_vectors_embed_return_and_keep_order() {
    let rows = tsv("embed_brd14.tsv");
    let mut pairs = Vec::new();
    for r in &rows {
        let id = embed(r[0].as_bytes()).unwrap_or_else(|e| panic!("{}: {e}", r[0]));
        assert_eq!(id.to_string(), r[2], "embedding of {}", r[0]);
        assert_eq!(unembed(id).unwrap(), r[0]);
        let (_, snow) = ecd_id::brd14_decode(r[0].as_bytes()).unwrap();
        assert_eq!(snow.to_string(), r[1]);
        pairs.push((r[0].clone(), id));
    }
    check_order_and_distinct(&pairs);
    println!("G8 brd14 vectors: {} embed, return and keep their order", rows.len());
}

fn check_order_and_distinct(pairs: &[(String, Id)]) {
    let mut by_text = pairs.to_vec();
    by_text.sort_by(|a, b| a.0.as_bytes().cmp(b.0.as_bytes()));
    by_text.dedup_by(|a, b| a.0 == b.0);
    let mut by_id = by_text.clone();
    by_id.sort_by_key(|p| p.1);
    assert!(by_text.iter().zip(&by_id).all(|(a, b)| a.0 == b.0), "order differs");
    let ids: HashSet<Id> = by_text.iter().map(|p| p.1).collect();
    assert_eq!(ids.len(), by_text.len(), "two brd14 texts share an embedding");
}

#[test]
fn g8_kan_corpus_embeds_distinct_in_order_with_case_pairs() {
    let rows = tsv("embed_kan.tsv");
    let mut pairs = Vec::new();
    for r in &rows {
        let id = embed(r[0].as_bytes()).unwrap();
        assert_eq!(id.to_string(), r[1]);
        assert_eq!(unembed(id).unwrap(), r[0]);
        pairs.push((r[0].clone(), id));
    }
    check_order_and_distinct(&pairs);
    let mut folded = std::collections::HashMap::<String, usize>::new();
    for (t, _) in &pairs {
        *folded.entry(t.to_ascii_lowercase()).or_default() += 1;
    }
    let case_groups = folded.values().filter(|&&n| n > 1).count();
    let case_ids: Vec<&(String, Id)> = pairs.iter().filter(|(t, _)| folded[&t.to_ascii_lowercase()] > 1).collect();
    let distinct: HashSet<Id> = case_ids.iter().map(|p| p.1).collect();
    assert!(case_groups > 0);
    assert_eq!(distinct.len(), case_ids.len());
    println!(
        "G8 KAN corpus: {} ids embed, distinct, in order, reversible; {} case groups ({} ids) embed as distinct values",
        pairs.len(),
        case_groups,
        case_ids.len()
    );
}

#[test]
fn g8_malformed_brd14_refused_before_embedding() {
    let rows = tsv("embed_invalid.tsv");
    for r in &rows {
        match embed(&unhex(&r[0])) {
            Ok(id) => panic!("{} embedded as {id}", r[0]),
            Err(e) => assert_eq!(e.reason(), r[1], "input {}", r[0]),
        }
    }
    let not = Id::parse(b"emq_job_23obzcbapvupu2272222").unwrap();
    assert_eq!(unembed(not), Err(Refusal::NotEmbedding));
    println!("G8 invalid: {} malformed brd14 texts refused with brd14's class", rows.len());
}

#[test]
fn g9_mxdb_identities() {
    let mut n = 0;
    for r in tsv("mxdb.tsv") {
        let bytes = unhex(&r[2]);
        if let Some(reason) = r[3].strip_prefix('!') {
            assert_eq!(Id::from_bytes(&bytes).unwrap_err().reason(), reason, "{} {}", r[0], r[2]);
            n += 1;
            continue;
        }
        let id = Id::parse(r[1].as_bytes()).unwrap();
        assert_eq!(hex(&id.to_bytes()), r[2]);
        let want_kind = match r[0].as_str() {
            "owner" => "sys",
            "store" => "sto",
            "database" => "dbs",
            "transaction" => "txn",
            other => panic!("field {other}"),
        };
        assert_eq!(id.kind().to_string(), want_kind);
        if r[0] == "owner" {
            assert_eq!(Id::system_id(id.system()).unwrap(), id);
            assert_eq!((id.time(), id.writer(), id.count()), (0, 0, 0));
        } else {
            assert!(id.time() > 0);
        }
        n += 1;
    }
    assert_eq!(hex(&Id::system_id(Code::parse("emq").unwrap()).unwrap().to_bytes()), "54ad8f60000000000000000000000000");
    println!("G9 mxdb: {n} rows");
}

#[test]
fn g9_no_lease_mints_time_zero() {
    let emq = Code::parse("emq").unwrap();
    assert_eq!(Lease::new(emq, 5, 0, 1000), Err(Refusal::LeaseWindow));
    let clock = Arc::new(ManualClock::new(0));
    let mut m = Minter::new(Lease::new(emq, 5, 1, 1000).unwrap(), clock, Arc::new(ManualPid::new(1)));
    let sys = m.mint(ecd_id::SYS).unwrap();
    assert_eq!(sys.time(), 1);
    assert_ne!(sys, Id::system_id(emq).unwrap());
    println!("G9: a lease starting at 0 is refused; a clock at 0 still stamps time 1, never the system's own id");
}

#[test]
fn g11_canonical_text_is_one_case() {
    assert!(ALPHABET.iter().all(|c| !c.is_ascii_uppercase()));
    let mut texts = Vec::new();
    for r in tsv("valid.tsv") {
        texts.push(r[0].clone());
    }
    for r in tsv("embed_kan.tsv") {
        texts.push(r[1].clone());
    }
    for r in tsv("embed_brd14.tsv") {
        texts.push(r[2].clone());
    }
    assert!(texts.iter().all(|t| !t.bytes().any(|c| c.is_ascii_uppercase())));
    let exact: HashSet<&String> = texts.iter().collect();
    let folded: HashSet<String> = texts.iter().map(|t| t.to_ascii_lowercase()).collect();
    assert_eq!(exact.len(), folded.len(), "two texts differ only by case");
    let upper = "emq_job_23obzcbapvupu2272222".to_ascii_uppercase();
    assert_eq!(Id::parse(upper.as_bytes()), Err(Refusal::Alphabet));
    println!("G11 one case: {} distinct texts, {} after case folding; uppercase refused", exact.len(), folded.len());
}

#[test]
fn registry_files_load() {
    let reg = repo_registry();
    let brd = Code::parse("brd").unwrap();
    assert_eq!(reg.system(brd).unwrap().state, ecd_id::State::Closed);
    assert!(reg.kinds_of(Code::parse("emq").unwrap()).count() >= 5);
    assert!(reg.kinds_of(brd).count() >= 36);
    println!("registry: {} systems load", reg.systems().count());
}

#[test]
fn registry_refusals() {
    let s = "code\tname\towner\thome\tstate\tserved_by\n";
    let k = "system\tkind\trole\tstate\tserved_by\tmeaning\n";
    let cases = [
        ("code\tname\n".to_owned(), k.to_owned(), "registry-header"),
        (format!("{s}emq\tEchoMQ\t-\t-\tlive\n"), k.to_owned(), "registry-row"),
        (format!("{s}3mq\tx\t-\t-\tlive\t-\n"), k.to_owned(), "registry-code"),
        (format!("{s}emq\tx\t-\t-\tlive\t-\nemq\ty\t-\t-\tlive\t-\n"), k.to_owned(), "registry-duplicate"),
        (format!("{s}emq\tx\t-\t-\tgone\t-\n"), k.to_owned(), "registry-state"),
        (format!("{s}emq\tx\t-\t-\tlive\tzzz\n"), k.to_owned(), "registry-served-by"),
        (format!("{s}emq\tx\t-\t-\tlive\t-\n"), format!("{k}kan\tdoc\tentity\tlive\t-\td\n"), "registry-system"),
        (format!("{s}emq\tx\t-\t-\tlive\t-\n"), format!("{k}emq\tsys\tentity\tlive\t-\td\n"), "registry-code"),
        (format!("{s}emq\tx\t-\t-\tlive\t-\n"), format!("{k}emq\tj2b\tentity\tlive\t-\td\n"), "registry-code"),
        (format!("{s}emq\tx\t-\t-\tlive\t-\n"), format!("{k}emq\tjob\tthing\tlive\t-\td\n"), "registry-role"),
    ];
    for (sys, kinds, reason) in &cases {
        let err = ecd_id::Registry::parse(sys, kinds).unwrap_err();
        assert_eq!(err.reason, *reason, "{sys:?} {kinds:?}");
    }
    println!("registry: {} faults refused", cases.len());
}
