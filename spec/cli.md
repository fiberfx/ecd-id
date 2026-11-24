# The command `eid`

`eid` parses and validates Ecd IDs and mints them from a shell. It is built from `go/cmd/eid` with the standard
library and the package `eid` only. Each call is one process: it holds no state of its own, and the only state it
keeps lives in a lease store ([lease.md](lease.md#file-store)).

## Commands

| Command | Does |
|---|---|
| `eid parse ID` | Prints the fields of a valid id as one JSON line. |
| `eid valid ID` | Prints `ok` when the id is valid. |
| `eid mint SYSTEM KIND [-n N] --store DIR [--home NAME] [--registry DIR]` | Takes a lease of SYSTEM from the store in DIR and mints N ids of KIND (default 1). |
| `eid mint SYSTEM KIND [-n N] --lease LEASE [--registry DIR]` | Mints N ids of KIND inside a lease given in its text form ([lease.md](lease.md#the-lease)). |
| `eid lease SYSTEM --store DIR [--home NAME] [--registry DIR] [--term MS] [--active-wait MS]` | Takes one lease of SYSTEM from the store in DIR and prints it in its text form ([lease.md](lease.md#the-lease)). |
| `eid detached KIND [-n N]` | Mints N detached ids of KIND ([lease.md](lease.md#detached)). |

`N` is a count from 1 to 1,000,000. A call that mints prints one id per line, in rising order, and prints nothing if it
refuses.

`parse` prints the fields of `vectors/valid.tsv` as a JSON object, in this order:

```json
{"text":"emq_job_23obzcbapvupu2272222","hex":"54acfa1c01a1fe839abbd57400140000","system":"emq","kind":"job","time":1795271400123,"writer":895287301,"count":0,"class":"registered","stream":"1795271400123-3050860491784323072"}
```

## Exit status and refusals

| Status | Meaning | Standard error |
|---|---|---|
| 0 | The command did what it names. | empty |
| 1 | The input or the state is refused for a reason of the specification. | `refused: REASON` |
| 2 | The command line is wrong: no command, a missing or extra argument, an unknown flag, a count, a term or a wait out of range, a system that is not three alphabet characters. | the cause, then the usage |
| 3 | The registry or the store cannot be read or written. | the cause |

The reasons are those of [id.md](id.md#strict-parse) for `parse` and `valid`, and for `mint` and `detached`:

| Check, in this order | Reason |
|---|---|
| The kind is three letters `a`–`z` | `kind` |
| The system is of the reserved class | `system-reserved` |
| A store is given and the system is of the detached class | `class` |
| The system has a row in the registry (a registered system only) | `unknown` |
| The system's state is `live` | `state` |
| The kind has a live row for the system | `kind` |
| The lease parses ([lease.md](lease.md#the-lease)) | `lease-syntax`, `lease-writer`, `lease-window` |
| The lease is of the system named | `lease-system` |
| A store: the allocator's start and grant ([lease.md](lease.md#allocator)) | `not-home`, `active`, `clock-behind`, `contended`, `second-full` |
| The lease is still open at the clock | `lease-ended` |

A detached system has no registry rows, so a lease of a detached system mints any kind of three letters `a`–`z`
([registry.md](registry.md#lease-free)).

## Minting from a store

One call is one allocator run:

1. It loads the registry and checks the system and the kind.
2. It opens the store in `DIR` and starts the allocator of the system under its home. The home is `--home`, else
   `EID_HOME`, else the host name up to its first dot, in lowercase. A store taken by another process makes the
   call retry for about three seconds, then refuse with `active`.
3. It asks for one lease of one hour and releases the store (`TakeLease`). A run that starts in the second of the
   previous grant waits for the next second (`wait` is retried twice) and then asks again.
4. It mints N ids inside that lease.

A system keeps one store, on one host; two hosts that keep a store of the same system can grant one writer twice.

## Taking a lease

`eid lease SYSTEM --store DIR` is one allocator run, as `mint --store` is, without the minting: it loads the registry and
checks the system, starts the allocator of the system under its home, takes one lease, releases the store and prints
the lease. The call is `TakeLease` of the Go package, which `mint --store` also uses.

- The output is one line, `system:writer:not_before:not_after`, the text that [`ParseLease`](lease.md#the-lease) accepts,
  for example `EID_LEASE=$(eid lease mxe --store "$EID_STORE")`. A refusal prints nothing on standard output.
- `--term MS` is the window of the lease, 1 to 86,400,000; the default is 86,400,000 (24 hours). A lease handed to a
  process at its start lasts its whole life when the process has no route to a fresh lease
  ([lease.md](lease.md#minter)), so the default is the longest term; `mint --store` keeps one hour.
- `--active-wait MS` is how long the call polls an activation that another run holds before it refuses with
  `active`, 0 to 60,000; the default is 3,000 (one attempt, then 100 more 30 ms apart).
- The command reads `EID_HOME` and `EID_REGISTRY` as `mint` does and does not read `EID_STORE`: a supervisor takes
  its three names from its own environment and passes them as flags ([registry.md](registry.md#systems)).
- `-n` and `--lease` are usage errors, as are `--term` and `--active-wait` for any other command.

The reasons, in this order; the home is checked before the store is opened, so a refusal at or before it creates no
file and no directory:

| Check, in this order | Reason |
|---|---|
| The system is of the reserved class | `system-reserved` |
| The system is of the detached class (a store never serves one) | `class` |
| The system has a row in the registry | `unknown` |
| The system's state is `live` | `state` |
| The row's home is the home (`--home`, else `EID_HOME`, else the host name) | `not-home` |
| The allocator's start and grant ([lease.md](lease.md#allocator)), after the retries of a held activation and of the grant second | `active`, `clock-behind`, `contended`, `second-full`, `clock-zero`, `wait` |

## Flags and environment

| Flag | Environment | Meaning |
|---|---|---|
| `--registry DIR` | `EID_REGISTRY` | The directory with `systems.tsv` and `kinds.tsv`. Needed for a registered system. |
| `--home NAME` | `EID_HOME` | The supervisor name the registry's `home` must equal. |
| `--term MS` | | `lease` only: the window of the lease in milliseconds, 1 to 86,400,000 (default 86,400,000). |
| `--active-wait MS` | | `lease` only: how long to poll a held activation before `active`, 0 to 60,000 (default 3,000). |

## Vectors

`vectors/cli.tsv` holds command lines with their exit status, standard output and standard error. Its rows assume the
clock reads 1795271400123 ms and `EID_REGISTRY` names `registry/`. The `mint --lease` rows pass leases that begin
after that clock, so the minted ids do not depend on the real time.
The `lease` rows name refusals that precede the store and usage errors; none of them opens `/tmp/eid-store`. A granted
lease depends on the clock and on a random offset, so its checks are tests, not rows.
