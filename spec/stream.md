# The Valkey stream-id mapping and its inverse

A Valkey stream entry id is two unsigned 64-bit numbers, written `<ms>-<seq>`. An Ecd ID maps to one:

```text
ms  = time                                        (bits 95–48, the Unix millisecond)
seq = system << 48 | writer << 18 | count         (15 + 30 + 18 = 63 bits; the top bit is always zero)
```

For `emq_job_23obzcbapvupu2272222` that is `1795271400123-3050860491784323072`.

## Why the tail carries the system

Two systems may append to one stream, for example two producers of `evt` records feeding one analytics stream. With a
tail of `writer << 18 | count` alone (bytes 10–15), two systems that hold the same writer number and mint in the same
millisecond at the same count produce the same stream id, and Valkey refuses the second append as not greater than the
stream's top. With the system in the tail:

- **Distinct.** Under the allocation premises ([lease.md](lease.md#premises)), (system, writer, time, count) is unique
  for every id, and the stream id keeps all four. Uniqueness does not need the kind: one minter's count never repeats a
  (time, count) pair across kinds.
- **Ordered by millisecond.** `ms` is the high half, so entries order by mint time first. Inside one millisecond they
  order by system, then writer, then count; that order is arbitrary at that resolution.
- **No big integer for time.** `ms` stays below 2^48. `seq` is above 2^53, so a JavaScript reader that parses the tail
  uses `BigInt`.

## The inverse

`from_stream(kind, ms, seq)` rebuilds the 16 bytes. The tail drops the kind, so the stream supplies it: a stream
carries ids of one kind. The inverse refuses:

| Input | Reason |
|---|---|
| The text is not two decimal numbers joined by one `-`, each without sign or leading zero and below 2^64 | `stream-syntax` |
| `ms` ≥ 2^48 or `seq` ≥ 2^63 | `stream-range` |
| The system in `seq` is of the reserved class | `system-reserved` |
| The kind is not three letters | `kind` |

## Appending

Valkey accepts `XADD key <ms>-<seq>` only when the id is greater than the stream's top entry (ms first, then seq) and
is not `0-0`. A producer that loses the race inside one millisecond to a higher tail is refused; it re-mints after its
clock reaches the next millisecond, and the new id is accepted. No client-minted scheme removes this refusal; the
mapping guarantees only that a refused append is a race, never a collision.

The receiving system's door refuses an id stamped further ahead of its own clock than its configured limit, so one
producer with a fast clock cannot hold a shared stream's top in the future ([lease.md](lease.md#door)).

## Vectors

`vectors/stream.tsv` gives, for each id, its kind, its stream id and the 48-bit tail without the system. Its first four
rows are four systems on writer 24576 in one millisecond at count 0: four distinct stream ids, one legacy tail.
`vectors/stream_invalid.tsv` gives the refusals.
