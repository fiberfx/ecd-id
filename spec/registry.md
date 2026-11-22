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
