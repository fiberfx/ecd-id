
### brd14 ids

The embedding of brd14 ids into ids of the system `brd` of [`../spec/registry.md`](../spec/registry.md#brd14). A
snowflake exceeds 2^53, so the payload is read through `bigint`.

| Name | Meaning |
|---|---|
| `fromBrd14(input)` | The id of a brd14 id: kind = the namespace in lowercase, time = `BRD14_EPOCH` + ts, writer = node, count = seq. The input is a `string`, read as UTF-8, or a `Uint8Array`. Reasons: `brd14-length`, `brd14-namespace`, `brd14-alphabet`, `brd14-range`. |
| `toBrd14(id)` | The inverse. Reasons: `brd14-system`, `brd14-time`, `brd14-writer`, `brd14-count`. |
| `fromBrd14Registered(input, brdKinds)`, `toBrd14Registered(id, brdKinds)` | The registry layer: `brdKinds` is the set of kinds that `registry/kinds.tsv` gives the system `brd`, supplied by the caller; any other kind is refused with `brd14-kind`. |
| `BRD14_LENGTH`, `BRD14_EPOCH` | The brd14 constants. |
# @fiberfx/eid

A TypeScript implementation of the Ecd ID (eid), version 1, as specified in [`../spec/id.md`](../spec/id.md): a
16-byte identifier of system, kind, mint millisecond, writer and count, with one 28-character lowercase text form such
as `emq_job_23obzcbapvupu2272222`. The vectors in [`../vectors/`](../vectors) are the conformance contract, shared with
the Go and Rust packages.

The package is private (`"private": true`) until a publish is decided. It has no runtime dependencies and uses only
standard JavaScript (ES2022, `BigInt`, `Uint8Array`, `TextEncoder`), so a browser, Node, Bun or Deno can import it.

## Import

The package exports TypeScript source; a runtime or bundler that reads `.ts` imports it directly.

```ts
import { parse, format, fromBytes, fromParts, valid, Refusal } from '@fiberfx/eid';

const id = parse('emq_job_23obzcbapvupu2272222');
id.system; // 'emq'
id.kind; // 'job'
id.ms; // 1795271400123
id.writer; // 895287301
id.count; // 0
id.class; // 'registered'
id.bytes; // Uint8Array(16)
String(id); // 'emq_job_23obzcbapvupu2272222'

try {
  parse('EMQ_JOB_23OBZCBAPVUPU2272222');
} catch (e) {
  if (e instanceof Refusal) e.reason; // 'alphabet'
}
```

Within this repository: `import { parse } from './ts/src/index.js'`.

## API

Every refusal throws a `Refusal`, whose `reason` is the text of the specification and the vectors. `valid` and
`validBinary` return a boolean instead.

### Ids

| Name | Meaning |
|---|---|
| `class Eid` | An id. `new Eid(bytes)` is binary ingress. Getters: `system`, `kind` (three characters), `systemCode`, `kindCode`, `reserved` (the two reserved bits, zero), `ms`, `writer`, `count`, `class`, `bytes` (a copy). Methods: `toString()` (the canonical text), `toJSON()`, `compare(other)`, `equals(other)`. |
| `parse(input)` | The strict parse of the canonical text, steps 1 to 7 of `spec/id.md#strict-parse`. The input is a `string`, read as UTF-8, or a `Uint8Array`, read as it is. Reasons: `length`, `separator`, `alphabet`, `pad`, `reserved`, `system-reserved`, `kind`. |
| `fromBytes(bytes)` | Binary ingress: exactly 16 bytes, then steps 5 to 7. Reasons: `length`, `reserved`, `system-reserved`, `kind`. |
| `format(id)` | The canonical text of an `Eid`, or of 16 bytes that pass binary ingress. |
| `valid(input)`, `validBinary(bytes)` | Whether the text or the 16 bytes would be accepted. |
| `fromParts({ system, kind, ms, writer, count })` | An id from its fields. `system` and `kind` are three characters or a 15-bit code. Reasons: `system-reserved`, `kind`, `range`, and `alphabet` for a malformed system string. |
| `systemId(system)` | The system's own id: kind `sys` and a zero local part. |
| `compare(a, b)` | Byte order, `-1`, `0` or `1`. |
| `parseCode(s)`, `codeToString(c)`, `codeClass(c)`, `isKind(c)`, `SYS` | The 15-bit codes: parse three alphabet characters (or `undefined`), print them, the class (`registered`, `detached`, `reserved`) from the first character, and whether a code is a valid kind. |
| `ALPHABET`, `TEXT_LENGTH`, `BYTE_LENGTH`, `TIME_MAX`, `WRITER_MAX`, `COUNT_LIMIT` | The layout constants. |

`ms` and `writer × 2^18 + count` are each below 2^53, so ids use plain numbers.

### Stream ids

The Valkey stream entry id of [`../spec/stream.md`](../spec/stream.md). `seq` exceeds 2^53, so a `StreamId` holds two
`bigint` values.

| Name | Meaning |
|---|---|
| `toStreamId(id)` | `{ ms, seq }` with `seq = system << 48 \| writer << 18 \| count`. |
| `fromStreamId(kind, sid)` | The inverse, given the kind the stream carries. Reasons: `kind`, `stream-range`, `system-reserved`. |
| `parseStreamId(text)`, `formatStreamId(sid)` | `<ms>-<seq>`, two decimals below 2^64 without sign or leading zero. Reason: `stream-syntax`. |
| `streamLess(a, b)`, `valkeyAccepts(top, entry)` | The order of stream ids, and Valkey's rule for `XADD` with an explicit id. |
| `legacyTail(id)` | The 48-bit tail without the system, kept as the control that shows why the stream id carries the system. |

### Leases (pure checks only)

The text form `system:writer:not_before:not_after` of [`../spec/lease.md`](../spec/lease.md).

| Name | Meaning |
|---|---|
| `parseLease(text)`, `formatLease(lease)` | The strict parse and the text form. Reasons: `lease-syntax`, `lease-system`, `lease-writer`, `lease-window`. |
| `newLease(system, writer, notBefore, notAfter)` | A lease, checked as the text form is checked. |
| `writerFor(second, index)` | `(second mod 2^17) × 2^13 + index`. |
| `LEASE_TERM_MAX`, `SECOND_SLOTS`, `INDEX_SLOTS` | The lease constants. |

## What it does not do

- It does not mint. There is no minter, clock, lease store, allocator, restart or clone guard, and no door; the Go and
  Rust packages hold these.
- It does not load the registry. The registry-aware checks (a system is registered and live, a kind has a row, a field's
  role) live in Go and Rust. The brd14 registry layer takes the kinds of `brd` from the caller. An id here is checked by layout and by kind only: three letters, with no registry lookup,
  so a lease-free id is not known to belong to a registered system or kind.
- It does not talk to Valkey. `valkeyAccepts` is the documented rule, not a connection.
- It has no command line and no file store.

## Tests

```sh
cd ts && bun test
```

The tests read `../vectors/{valid,invalid,binary_invalid,lease,stream,stream_invalid,brd14}.tsv`, and the brd14 test
reads `../registry/kinds.tsv`. Each test asserts that the
number of rows it checked equals the number of data rows in the file, and prints `VECTORS <file> checked N of M`. Further
tests give one valid and one refused case for each function where the vectors leave a gap. `bun test` strips types
without checking them; `tsc --noEmit -p tsconfig.json` checks `src/` under `strict`.

`../rows/eid-ts.rows.sh` runs the tests, the brd14 test on its own, the typecheck and three planted mutants (two alphabet
characters swapped; the pad check removed; writer and count swapped in `fromBrd14`), each of which must make the tests fail.
