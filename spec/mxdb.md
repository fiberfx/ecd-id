# mxdb's 16-byte identities

mxdb treats four identities as 16 opaque bytes that the engine never parses. All four fit this format without changing
a key length.

| mxdb field | mxdb's rule | In this format |
|---|---|---|
| `store_id` | The caller's identity of the store; not all zero | An Ecd ID of kind `sto`, minted by the owning system when it creates the store |
| database id | The catalog's key: exactly 16 bytes, not all zero | An Ecd ID of kind `dbs`, minted by the owning system |
| `owner` | In the manifest; all zero means none | The system's own id: its code, kind `sys` and a zero local part |
| `lineage` | 16 random bytes the engine draws when the file is created | **Not an Ecd ID.** It names a file's birth, which no authority grants; it stays the engine's own random bytes |

## The system's own id

`<code>_sys_22222222222222222222`: the system code, kind `sys`, reserved bits, time, writer and count all zero. For
example `emq_sys_22222222222222222222` is `54ad8f60` followed by 12 zero bytes.

- It is derived, not minted, so it exists before the system's first lease, and it survives restart and movement because
  the code does.
- No minted id equals it: every lease starts at time 1 or later ([lease.md](lease.md#the-lease)).
- It sorts before every other id of its system's `sys` kind.
- A detached system has no unique code (5,120 codes are shared by chance), so its owner field holds a minted `sys` id
  kept with its code instead.

## Derived names

A name derived rather than minted has time 0. Its writer and count come from a 48-bit hash of what it names. The hash
and the order of its bits are the deriving system's own; the writer takes the high 30 bits and the count the low 18. No
minted id equals it, since every lease starts at time 1 or later ([lease.md](lease.md#the-lease)). The system's own id
is the derived name of kind `sys` whose hash is zero.

## "None"

Sixteen zero bytes are never a valid Ecd ID: their system starts with `2`, which the parse refuses (`system-reserved`).
mxdb's "none" therefore cannot be mistaken for an id, and an all-zero store or database id is refused.

## Ids in keys

An id inside an application key is 16 bytes, never 18, since the kind is inside. A table of one kind minted by one
owner appends at its right edge; a table of mixed kinds keyed by the raw id has one right edge per kind.

## Scope

Version 1 supplies the encodings and their vectors (`vectors/mxdb.tsv`). Opening a store with an owner, and the
engine's own checks, belong to mxdb.
