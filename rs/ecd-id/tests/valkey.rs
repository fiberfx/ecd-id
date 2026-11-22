//! Stream ids against a running Valkey server, when `ECD_ID_VALKEY` names its address. Without it the test prints
//! that it did not run and passes; the arithmetic is covered by tests/stream.rs.

mod common;

use std::io::{BufRead, BufReader, Write};
use std::net::TcpStream;
use std::sync::Arc;
use std::time::Duration;

use ecd_id::{Code, Id, Lease, ManualClock, ManualPid, Minter, StreamId, from_stream};

#[derive(Debug)]
#[allow(dead_code)]
enum Reply {
    Simple(String),
    Error(String),
    Int(i64),
    Bulk(Option<Vec<u8>>),
    Array(Option<Vec<Reply>>),
}

struct Conn {
    w: TcpStream,
    r: BufReader<TcpStream>,
}

impl Conn {
    fn open(addr: &str) -> Conn {
        let w = TcpStream::connect(addr).unwrap_or_else(|e| panic!("connect {addr}: {e}"));
        w.set_read_timeout(Some(Duration::from_secs(10))).unwrap();
        let r = BufReader::new(w.try_clone().unwrap());
        Conn { w, r }
    }

    fn cmd(&mut self, args: &[&str]) -> Reply {
        let mut out = format!("*{}\r\n", args.len()).into_bytes();
        for a in args {
            out.extend_from_slice(format!("${}\r\n", a.len()).as_bytes());
            out.extend_from_slice(a.as_bytes());
            out.extend_from_slice(b"\r\n");
        }
        self.w.write_all(&out).unwrap();
        self.read()
    }

    fn line(&mut self) -> String {
        let mut s = String::new();
        self.r.read_line(&mut s).unwrap();
        s.trim_end_matches("\r\n").to_owned()
    }

    fn read(&mut self) -> Reply {
        let l = self.line();
        let (tag, rest) = l.split_at(1);
        match tag {
            "+" => Reply::Simple(rest.to_owned()),
            "-" => Reply::Error(rest.to_owned()),
            ":" => Reply::Int(rest.parse().unwrap()),
            "$" => {
                let n: i64 = rest.parse().unwrap();
                if n < 0 {
                    return Reply::Bulk(None);
                }
                let mut buf = vec![0u8; n as usize + 2];
                std::io::Read::read_exact(&mut self.r, &mut buf).unwrap();
                buf.truncate(n as usize);
                Reply::Bulk(Some(buf))
            }
            "*" => {
                let n: i64 = rest.parse().unwrap();
                if n < 0 {
                    return Reply::Array(None);
                }
                Reply::Array(Some((0..n).map(|_| self.read()).collect()))
            }
            other => panic!("unexpected reply {other}{rest}"),
        }
    }
}

fn bulk(r: &Reply) -> String {
    match r {
        Reply::Bulk(Some(b)) => String::from_utf8_lossy(b).into_owned(),
        other => panic!("expected a bulk string, got {other:?}"),
    }
}

const T: u64 = 1_795_357_800_000;

fn minter(system: &str, clock: Arc<ManualClock>) -> Minter {
    let lease = Lease::new(Code::parse(system).unwrap(), 3 << 13, T - 5000, T - 5000 + 86_400_000).unwrap();
    Minter::new(lease, clock, Arc::new(ManualPid::new(1)))
}

#[test]
fn g4_valkey_live() {
    let Ok(addr) = std::env::var("ECD_ID_VALKEY") else {
        println!("VALKEY-NOT-RUN: ECD_ID_VALKEY is not set");
        return;
    };
    let mut c = Conn::open(&addr);
    let server = match c.cmd(&["INFO", "server"]) {
        Reply::Bulk(Some(b)) => {
            let info = String::from_utf8_lossy(&b).into_owned();
            let pick = |key: &str| info.lines().find(|l| l.starts_with(key)).map(str::to_owned);
            pick("valkey_version:").or_else(|| pick("redis_version:")).unwrap_or_else(|| "version unknown".to_owned())
        }
        other => format!("{other:?}"),
    };
    for k in ["ecdid:g4", "ecdid:g4:legacy", "ecdid:g4:race"] {
        c.cmd(&["DEL", k]);
    }

    // Two systems on one writer, one evt each per millisecond, appended in stream-id order.
    let clock = Arc::new(ManualClock::new(T));
    let (mut a, mut b) = (minter("emq", clock.clone()), minter("kan", clock.clone()));
    let evt = Code::parse("evt").unwrap();
    let mut ids: Vec<Id> = Vec::new();
    for i in 0..500 {
        clock.set(T + i);
        ids.push(a.mint(evt).unwrap());
        ids.push(b.mint(evt).unwrap());
    }
    ids.sort_by_key(|i| i.stream_id());
    for id in &ids {
        let sid = id.stream_id().to_string();
        let reply = c.cmd(&["XADD", "ecdid:g4", sid.as_str(), "id", id.to_string().as_str()]);
        assert_eq!(bulk(&reply), sid, "XADD {sid}");
    }
    let Reply::Array(Some(entries)) = c.cmd(&["XRANGE", "ecdid:g4", "-", "+"]) else { panic!("XRANGE") };
    assert_eq!(entries.len(), ids.len());
    for (e, id) in entries.iter().zip(&ids) {
        let Reply::Array(Some(parts)) = e else { panic!("entry") };
        let sid = StreamId::parse(&bulk(&parts[0])).unwrap();
        assert_eq!(from_stream("evt", sid).unwrap(), *id);
        let Reply::Array(Some(fields)) = &parts[1] else { panic!("fields") };
        assert_eq!(bulk(&fields[1]), id.to_string());
    }

    // Control: the tail without the system. The second system's entry in each millisecond collides.
    let mut legacy_refused = 0;
    for id in &ids {
        let sid = format!("{}-{}", id.time(), id.legacy_tail());
        if let Reply::Error(_) = c.cmd(&["XADD", "ecdid:g4:legacy", sid.as_str(), "id", id.to_string().as_str()]) {
            legacy_refused += 1;
        }
    }

    // A lower tail loses the race inside one millisecond and is accepted once re-minted in the next.
    clock.set(T + 10_000);
    let (mut low, mut high) = (minter("emq", clock.clone()), minter("kan", clock.clone()));
    let h = high.mint(evt).unwrap().stream_id().to_string();
    let l = low.mint(evt).unwrap().stream_id().to_string();
    assert_eq!(bulk(&c.cmd(&["XADD", "ecdid:g4:race", h.as_str(), "f", "v"])), h);
    let race = c.cmd(&["XADD", "ecdid:g4:race", l.as_str(), "f", "v"]);
    assert!(matches!(race, Reply::Error(_)), "same-ms lower tail must be refused, got {race:?}");
    clock.advance(1);
    let again = low.mint(evt).unwrap().stream_id().to_string();
    assert_eq!(bulk(&c.cmd(&["XADD", "ecdid:g4:race", again.as_str(), "f", "v"])), again);

    if let Reply::Error(e) = c.cmd(&["XADD", "ecdid:g4:race", "0-0", "f", "v"]) {
        println!("VALKEY 0-0 refused: {e}");
    }
    println!(
        "VALKEY-RAN {server}: {} entries from 2 systems on one writer accepted in ms order and mapped back; legacy tail refused {legacy_refused}; race {l} refused, re-minted {again} accepted",
        entries.len()
    );
    assert_eq!(legacy_refused, ids.len() / 2);
}
