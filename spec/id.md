# The Ecd ID, version 1: layout, alphabet, text and strict parse

An Ecd ID is 16 bytes. Read from the most significant bit, it names the system that minted it, the kind of thing it
names, and a local part: the Unix millisecond of the mint, the writer that minted it, and a count inside that writer's
millisecond. Its one text form is 28 lowercase characters, for example `emq_job_23obzcbapvupu2272222`.

## Binary layout

| Bits (from the top) | Width | Field | Meaning |
|---|---|---|---|
| 127–113 | 15 | system | The system that minted the id: three alphabet characters. |
| 112–98 | 15 | kind | The kind of thing, scoped to the system: three letters. |
| 97–96 | 2 | reserved | Zero. Any other value is refused. |
| 95–48 | 48 | time | Unix milliseconds stamped by the minter, to the year 10889. |
| 47–18 | 30 | writer | The writer number of the lease the minter holds. |
| 17–0 | 18 | count | A counter inside one writer's millisecond: 262,144 ids per millisecond per writer. |

- Bytes 0–3 hold system, kind and the reserved bits; bytes 4–9 hold the millisecond, big-endian; bytes 10–15 hold
  `writer × 2^18 + count`, big-endian.
- Comparing the 16 bytes compares system, then kind, then time, then writer, then count.
- Time and `writer × 2^18 + count` are each 48 bits, below 2^53, so neither needs a big integer in JavaScript.
- An Ecd ID is not a UUID. It has no version or variant bits and does not belong in a field that claims them.

### What byte order means

| Scope | A later id compares greater? |
|---|---|
| One system, one kind, one minter | Always: time never goes back, the count rises, a burst borrows the next millisecond, and a fresh lease starts after the minter's last id ([lease.md](lease.md#minter)). |
| One system, one kind, several minters | Once the gap exceeds the skew between their clocks. Inside one millisecond the order is by writer number. |
| One system, several kinds | No. Kind sorts before time, so each kind is one contiguous byte range with its own right edge. |
| Several systems | The system half sorts by its code's characters only: reserved, then detached, then registered, then alphabet order. Order across systems carries no meaning. |

## System classes

The first character of the system code decides its class:

| First character | Class | Codes |
|---|---|---|
| a letter `a`–`z` | **registered**: a row in `registry/systems.tsv` | 26 × 32 × 32 = 26,624 |
| a digit `3`–`7` | **detached**: drawn at random by a producer that holds no lease ([lease.md](lease.md#detached)) | 5 × 32 × 32 = 5,120 |
| the digit `2` (value zero) | **reserved** for a later format; refused | 1,024 |

A kind is three letters `a`–`z`, 17,576 per system. A digit in a kind is refused. Every detached id sorts below every
registered id.

## Alphabet

`234567abcdefghijklmnopqrstuvwxyz`, values 0 to 31 in that order: the RFC 4648 Base32 symbols in ASCII order.

- It has all 26 letters, so a system code and a kind are their own three-character names, and no codec needs a registry
  to print or parse an id.
- ASCII order equals value order, so sorting texts sorts ids.
- One case only. A valid text never contains an uppercase letter, so no two valid texts differ only by letter case,
  and a text is safe as a file name on a case-folding file system.
- No `0` and no `1`: nothing to confuse with `o` or `l`.
- It differs from Base32hex (`0-9a-v`), the alphabet of xid and of several other identifier formats, so their
  decoders cannot half-parse an Ecd ID.

## Canonical text

Encode the 16 bytes most significant bit first in 5-bit groups: 26 characters, the last carrying 3 data bits and 2 zero
pad bits. Write `_` after the 3rd and the 6th character. The result is exactly 28 bytes:

```text
emq_job_23obzcbapvupu2272222      54acfa1c 01a1fe839abb d57400140000
└┬┘ └┬┘ └────────┬─────────┘      system emq, kind job, time 1795271400123, writer 895287301, count 0
sys kind  time, writer, count     the same writer's next id in that millisecond: emq_job_23obzcbapvupu2272226
```

Text order equals byte order (`vectors/valid.tsv`).

## Strict parse

The parse runs on bytes, in this order. Each step refuses with its own reason; none repairs.

| Step | Check | Reason |
|---|---|---|
| 1 | The input is exactly 28 bytes. | `length` |
| 2 | Bytes 3 and 7 are `_`. | `separator` |
| 3 | The other 26 bytes are in the alphabet. No case folding, no alias table, no skipped whitespace. | `alphabet` |
| 4 | The two pad bits are zero: the last character is one of `26cgkosw`. | `pad` |
| 5 | The reserved bits are zero. | `reserved` |
| 6 | The system's first character is not `2`. | `system-reserved` |
| 7 | The kind is three letters. | `kind` |
| 8 | Where a field declares a role, the role's own check runs ([registry.md](registry.md#roles)). | the caller's |

- Steps 1–7 need no registry. Binary ingress runs steps 5–7 on the 16 bytes, after checking that there are exactly 16
  (`length`).
- An embedded `\r`, `\n`, space or NUL fails step 3 when the input is 28 bytes and step 1 otherwise. A non-ASCII
  character fails step 3 or step 1 the same way.
- Sixteen zero bytes are never a valid id: their system starts with `2`, which step 6 refuses.
- Encoding is a one-to-one map between valid 16-byte values and valid texts, and the strict parse is its inverse.

## What the text is not

- It has no checksum: one changed character can be another valid id.
- It is not a W3C trace id, which keeps its own 32-hex form under its own type. A random 16-byte value passes steps
  5–7 about one time in eight, so a typed field, not a parse, tells an Ecd ID from a trace id.
- An id shows its system, kind, mint millisecond and writer to anyone who holds it. A value that must not leak them,
  such as a share link or a session secret, is an opaque random token that maps to an id; it is not an Ecd ID.

## Vectors

`vectors/valid.tsv` (valid ids, both ways), `vectors/invalid.tsv` (texts with the reason of the first failing step) and
`vectors/binary_invalid.tsv` (16-byte inputs refused at binary ingress).
