# The registries

Two registries exist, both tab-separated files in this repository: `registry/systems.tsv` and `registry/kinds.tsv`.
Nobody runs them; they are data, and their history is the repository's history.

## Systems

```text
code  name  owner  home  state  served_by
```

- One row per registered system. A code is three alphabet characters whose first is a letter; it is never reused and
  never renamed. A service rename changes `name`, not `code`.
- `home` names the one supervisor that may start the system's allocator ([lease.md](lease.md#allocator)); `-` means
  none is assigned, and no allocator starts.
- `state` is `live`, `closed` (ids stay readable; nothing mints) or `retired`.
- `served_by` is `-` or the code of a system that now serves this one, after a merge.
- Detached codes have no rows.

## Kinds

```text
system  kind  role  state  served_by  meaning
```

- One row per (system, kind). The system's owner adds rows for its own system. Kinds are scoped to their system, so
  `msg` of `emq` (a bus message) and `msg` of another system never meet.
- `kind` is three letters `a`–`z`.
- `served_by` is `-` or the code of the system that now serves this kind, when a whole kind changes owner.
- The kind `sys` belongs to the format: in every system it names the system itself ([mxdb.md](mxdb.md)). It has no
  row, and a row for it is refused.

<a id="roles"></a>

## Roles

`role` is one of `entity`, `activation`, `command`, `run`, `stream`, `store`, `database`, `transaction`,
`correlation` and `request`; the format's `sys` has the role `system`. A typed field checks the role, so a `command` id
cannot stand where a `run` is expected.

<a id="lease-free"></a>

## Lease-free ids are checked by kind only

- A detached id (first character `3`–`7`, [id.md](id.md#system-classes)) has no registry rows.
- Its kind must be three letters `a`–`z`, refused with `kind` otherwise. Where a kind is expected, its kind name is
  compared with the expected kinds.
- The registry is not consulted for it, and no role is checked.
- The check of the system and of the (system, kind) row against the registry, and a typed field's role check
  ([Roles](#roles)), apply to ids of registered-class systems.

## Classes and retirement

- The class of a system is read from its code's first character ([id.md](id.md#system-classes)), not from the file.
- A kind or a system is closed or retired, never deleted and never reassigned.
- Retiring a system or kind changes its `state`; ids already minted stay valid and readable.

## Loading

A loader reads both files strictly and refuses:

| Fault | Reason |
|---|---|
| A header that is not exactly the column list above | `registry-header` |
| A row with the wrong number of fields, or an empty field | `registry-row` |
| A system code that is not registered-class, or a kind that is not three letters, or a `sys` row | `registry-code` |
| A repeated code, or a repeated (system, kind) | `registry-duplicate` |
| A state outside `live`, `closed`, `retired` | `registry-state` |
| A role outside the list | `registry-role` |
| A kind row for a system that has no row | `registry-system` |
| A `served_by` that names no system | `registry-served-by` |

Lines starting with `#` are comments. No codec carries a copy of the registry: the parse needs none, since the names
are the numbers. Generating per-language kind constants from these files is out of scope for version 1.

## The system `brd`

`brd` is a registered system with no home and state `retired`. Its row and its kinds stay in the files, its ids stay
valid and readable, and nothing mints under it; the code is never reassigned.

<a id="brd14"></a>

### Embedded brd14 ids

An id of the system `brd` arises only by embedding a brd14 id, the 14-byte identifier that preceded this format.
Nothing was ever minted under `brd`, so every `brd` id is an embedding. The embedding is deterministic and
stateless and needs no table: a stored brd14 id is migrated by computing its Ecd ID, and the Ecd ID gives the brd14 id
back.

A brd14 id is 14 ASCII bytes: a namespace `NS` of three bytes `A`–`Z`, then a payload of 11 Base62 digits. The Base62
alphabet is `0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz`, values 0 to 61 in that order, so byte
order equals value order. The payload is fixed-width, most significant digit first and left-padded with `0`; it encodes
a 63-bit snowflake, below 2^63, whose largest payload is `AzL8n0Y58m7`:

```text
snowflake = ts << 22 | node << 12 | seq     ts: 41 bits, milliseconds since 1704067200000 (2024-01-01T00:00:00Z)
                                            node: 10 bits, the minting node; seq: 12 bits, the count in its millisecond
```

| brd14 id | Ecd ID |
|---|---|
| (none) | system `brd` |
| `NS` | kind: `NS` in lowercase |
| `ts` | time: `1704067200000 + ts` |
| `node` | writer: `node` |
| `seq` | count: `seq` |

`from_brd14(text)` refuses, checking in this order (the order of brd14's own decode):

| Input | Reason |
|---|---|
| Not exactly 14 bytes | `brd14-length` |
| A namespace byte outside `A`–`Z`; a namespace with a digit or a lowercase letter is not embeddable | `brd14-namespace` |
| A payload byte outside the Base62 alphabet | `brd14-alphabet` |
| A payload of 2^63 or more, that is, above `AzL8n0Y58m7` | `brd14-range` |

`to_brd14(id)` refuses, checking in this order:

| Input | Reason |
|---|---|
| The system is not `brd` | `brd14-system` |
| The time is below 1704067200000, or at or above 1704067200000 + 2^41 | `brd14-time` |
| The writer is 2^10 or more | `brd14-writer` |
| The count is 2^12 or more | `brd14-count` |

The kind needs no check: the kind of a valid id is three letters, and their uppercase form is a namespace.

#### The registry layer

`from_brd14` and `to_brd14` need no registry, as steps 1–7 of the strict parse need none ([id.md](id.md#strict-parse)).
A registry layer runs the same function first and then refuses an id whose kind has no row of the system `brd` in
`registry/kinds.tsv`, with the reason `brd14-kind`. A row in any state counts: a closed or retired kind stays readable.
The namespace `SYS` embeds into the kind `sys`, which belongs to the format and has no row, so the registry layer
refuses it.

#### Order and inverse

- Within one namespace, the byte order of the embedded ids equals the numeric order of the snowflakes: `ts`, then
  `node`, then `seq`, which is brd14's mint order. Across namespaces, it equals the byte order of the brd14 texts, since
  `A`–`Z` and `a`–`z` are in the same order.
- `to_brd14(from_brd14(x)) = x` for every brd14 id `x`, so the embedding is injective, and `from_brd14(to_brd14(y)) = y`
  for every id `y` that `to_brd14` accepts. Those ids are exactly the `brd` ids with time in
  [1704067200000, 1704067200000 + 2^41), writer below 2^10 and count below 2^12.

`vectors/brd14.tsv` gives both directions, every refusal and, beside each, the registry layer's answer.
