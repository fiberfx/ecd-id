# fiberfx-eid

A Python implementation of the Ecd ID (eid), version 1, as specified in [`../spec/id.md`](../spec/id.md): a 16-byte
identifier of system, kind, mint millisecond, writer and count, with one 28-character lowercase text form such as
`emq_job_23obzcbapvupu2272222`. The vectors in [`../vectors/`](../vectors) are the conformance contract, shared with
the Go, Rust and TypeScript packages.

The distribution is `fiberfx-eid`, version 0.10.0 with the repository, and the package is `eid`. It carries the
classifier `Private :: Do Not Upload` until a publish is decided. It needs Python 3.12 or later and uses only the
standard library.

## Import

```python
import eid
from eid import Refusal, parse

x = parse("emq_job_23obzcbapvupu2272222")
x.system  # 'emq'
x.kind  # 'job'
x.ms  # 1795271400123
x.writer  # 895287301
x.count  # 0
x.class_  # 'registered'
x.bytes  # 16 bytes
str(x)  # 'emq_job_23obzcbapvupu2272222'

try:
    parse("EMQ_JOB_23OBZCBAPVUPU2272222")
except Refusal as e:
    e.reason  # 'alphabet'
```

Within this repository: `PYTHONPATH=py/src python3 -c 'import eid'`, or `pip install ./py`.

## API

Every refusal raises a `Refusal`, a `ValueError` whose `reason` and message are the text of the specification and the
vectors. `valid` and `valid_binary` return a boolean instead. An input of the wrong type (a number where text is
expected, a `str` where bytes are expected) raises `TypeError`, not a `Refusal`.

### Ids

| Name | Meaning |
|---|---|
| `class Eid` | An id: immutable, hashable, and ordered by its bytes. `Eid(data)` is binary ingress. Properties: `system`, `kind` (three characters), `system_code`, `kind_code`, `reserved` (the two reserved bits, zero), `ms`, `writer`, `count`, `class_`, `bytes`. `str(x)` is the canonical text, `bytes(x)` the 16 bytes; `==`, `<`, `<=`, `>`, `>=` compare in byte order. |
| `parse(text)` | The strict parse of the canonical text, steps 1 to 7 of `spec/id.md#strict-parse`. The input is a `str`, read as UTF-8, or a bytes-like object, read as it is. Reasons: `length`, `separator`, `alphabet`, `pad`, `reserved`, `system-reserved`, `kind`. |
| `from_bytes(data)` | Binary ingress: exactly 16 bytes, then steps 5 to 7. Reasons: `length`, `reserved`, `system-reserved`, `kind`. |
| `format(eid)` | The canonical text of an `Eid`, or of 16 bytes that pass binary ingress. |
| `valid(text)`, `valid_binary(data)` | Whether the text or the 16 bytes would be accepted. |
| `from_parts(system, kind, ms, writer, count)` | An id from its fields. `system` and `kind` are three characters or a 15-bit code. Reasons: `system-reserved`, `kind`, `range` (a code or field that is not an `int` in its range), and `alphabet` for a malformed system string. |
| `system_id(system)` | The system's own id: kind `sys` and a zero local part. |
| `compare(a, b)` | Byte order, `-1`, `0` or `1`. |
| `parse_code(s)`, `code_to_string(c)`, `code_class(c)`, `is_kind(c)`, `SYS` | The 15-bit codes: parse three alphabet characters (or `None`), print them, the class (`registered`, `detached`, `reserved`) from the first character, and whether a code is a valid kind. |
| `ALPHABET`, `TEXT_LENGTH`, `BYTE_LENGTH`, `TIME_MAX`, `WRITER_MAX`, `COUNT_LIMIT` | The layout constants. |
| `Code`, `SystemClass`, `Reason` | Type aliases: a code is an `int`; the class and the reason are `Literal` strings. |

Fields and codes are Python `int` values, with no 2^53 limit.

### Stream ids

The Valkey stream entry id of [`../spec/stream.md`](../spec/stream.md).

| Name | Meaning |
|---|---|
| `StreamId(ms, seq)` | A named tuple of two `int` values; tuple order is stream order. |
| `to_stream_id(eid)` | `StreamId(ms, seq)` with `seq = system << 48 \| writer << 18 \| count`. |
| `from_stream_id(kind, sid)` | The inverse, given the kind the stream carries. Reasons: `kind`, `stream-range` (also for a negative number), `system-reserved`. |
| `parse_stream_id(text)`, `format_stream_id(sid)` | `<ms>-<seq>`, two ASCII decimals below 2^64 without sign or leading zero. Reason: `stream-syntax`. |
| `stream_less(a, b)`, `valkey_accepts(top, entry)` | The order of stream ids, and Valkey's rule for `XADD` with an explicit id; `top` is `None` for an empty stream. |
| `legacy_tail(eid)` | The 48-bit tail without the system, kept as the control that shows why the stream id carries the system. |

### Leases (pure checks only)

The text form `system:writer:not_before:not_after` of [`../spec/lease.md`](../spec/lease.md).

| Name | Meaning |
|---|---|
| `parse_lease(text)`, `format_lease(lease)` | The strict parse and the text form. Reasons: `lease-syntax`, `lease-system`, `lease-writer`, `lease-window`. |
| `new_lease(system, writer, not_before, not_after)` | A lease, checked as the text form is checked: a number that is not an `int` from 0 to 2^64 - 1 is refused with `lease-syntax`. |
| `Lease` | A frozen data class with the fields `system`, `writer`, `not_before`, `not_after`. |
| `writer_for(second, index)` | `(second mod 2^17) × 2^13 + index mod 2^13`. Reason: `range` for an argument that is not a non-negative `int`. |
| `LEASE_TERM_MAX`, `SECOND_SLOTS`, `INDEX_SLOTS` | The lease constants. |

### brd14 ids

The embedding of brd14 ids into ids of the system `brd` of [`../spec/registry.md`](../spec/registry.md#brd14).

| Name | Meaning |
|---|---|
| `from_brd14(text)` | The id of a brd14 id: kind = the namespace in lowercase, time = `BRD14_EPOCH` + ts, writer = node, count = seq. The input is a `str`, read as UTF-8, or a bytes-like object. Reasons: `brd14-length`, `brd14-namespace`, `brd14-alphabet`, `brd14-range`. |
| `to_brd14(eid)` | The inverse. Reasons: `brd14-system`, `brd14-time`, `brd14-writer`, `brd14-count`. |
| `from_brd14_registered(text, brd_kinds)`, `to_brd14_registered(eid, brd_kinds)` | The registry layer: `brd_kinds` is a container of the kinds that `registry/kinds.tsv` gives the system `brd`, supplied by the caller; any other kind is refused with `brd14-kind`. |
| `BRD14_LENGTH`, `BRD14_EPOCH` | The brd14 constants. |

## What it does not do

- It does not mint. There is no minter, clock, lease store, allocator, restart or clone guard, and no door; the Go and
  Rust packages hold these. `vectors/mint.tsv` is therefore outside its scope.
- It does not load the registry. The registry-aware checks (a system is registered and live, a kind has a row, a
  field's role) live in Go and Rust. The brd14 registry layer takes the kinds of `brd` from the caller. An id here is
  checked by layout and by kind only: three letters, with no registry lookup, so a lease-free id is not known to belong
  to a registered system or kind.
- It has no mxdb identities and no command line: `vectors/mxdb.tsv` and `vectors/cli.tsv` are outside its scope.
- It does not talk to Valkey. `valkey_accepts` is the documented rule, not a connection.

## Tests

```sh
cd py && PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=src python3 -m unittest discover -s tests -t . -v
cd py && PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=src python3 -m unittest tests.test_brd14 -v
```

The tests use `unittest` only and read
`../vectors/{valid,invalid,binary_invalid,lease,stream,stream_invalid,brd14}.tsv`; the brd14 test also reads
`../registry/kinds.tsv`. Each vector test asserts that the number of rows it checked equals the number of data rows
in the file and prints `VECTORS <file> <checked>/<rows>` on a line of its own to standard error;
the brd14 order test prints a line that starts with `BRD14`. Further tests in `tests/test_api.py` give one valid and
one refused case for each function where the vectors leave a gap, and check the Python surface of an `Eid`
(immutability, hashing, ordering, pickling).

`../rows/eid-py.rows.sh` runs the tests, the brd14 test on its own and planted mutants (two alphabet characters swapped;
the pad check removed; writer and count swapped in `from_brd14`), each of which must make the tests fail. Its row
`eid-py-tampered-vector` changes one hex digit of the bytes column in the first data row of `../vectors/valid.tsv` in a
scratch copy; the tests must fail on that copy.
