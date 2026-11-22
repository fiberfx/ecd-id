# Leases, the allocator, the minter and the door

A producer mints ids with no round trip per id. It holds a **lease** from its system: one writer number and a window
of id-time it alone may stamp. Inside the lease it mints locally; the allocator is asked once per process start and
then about every 12 hours.

<a id="premises"></a>

## Premises

Two ids of one registered system are never equal while three premises hold:

- **P1, one allocator:** at most one allocator serves a system code at a time.
- **P2, no second twice:** an allocator never grants from a clock second it granted from in an earlier run.
- **P3, stay inside the lease:** one lease feeds one minter state; the minter stamps only id-times inside its lease and
  never repeats a (time, count) pair.

Two ids are equal only if they agree in system, writer, time and count. P1 and P2 make (writer, id-time window)
exclusive; P3 makes (time, count) unique inside it. No agreement between clocks is needed: a minter with a wrong clock
still stamps inside its own window.

## The lease

`lease(system) → (system, writer, not_before, not_after)`.

- **Text form**, for passing a lease to a process in its environment: `ECD_ID_LEASE=emq:895287301:1795271400000:1795357800000`,
  that is `system:writer:not_before:not_after` in decimal milliseconds. The parse is strict:

| Check | Reason |
|---|---|
| Four fields separated by `:`; a system of three alphabet characters; decimal numbers without sign or leading zero | `lease-syntax` |
| The system is not of the reserved class | `lease-system` |
| `writer` < 2^30 | `lease-writer` |
| `not_before` ≥ 1; `not_after` ≥ `not_before`; `not_after` < 2^48; `not_after − not_before` ≤ 24 hours | `lease-window` |

- `not_before` ≥ 1 means no lease covers time zero, so no minted id equals a system's own id, whose local part is zero
  ([mxdb.md](mxdb.md)).
- One lease goes to one process.

## Writer numbers

`writer = (grant second mod 2^17) × 2^13 + index`

- The 13-bit index counts grants inside that second, starting from an offset drawn at random when the allocator starts:
  8,192 grants per second per system.
- `not_before` is the grant second in milliseconds; `not_after` is at most 24 hours later.
- A writer number comes round again after 2^17 seconds, 36.4 hours. The earlier lease on it ended at least 12.4 hours
  before the later one begins, so two leases on one writer never share an id-time.

<a id="allocator"></a>

## The allocator

The allocator's whole durable state is **one number per system**, `last_second`, kept in the system's own store and
changed only by compare-and-set. In memory it holds the second of its last grant, the number of grants in that second,
and its random offset.

**Start.** An allocator starts only if all of these hold, and refuses otherwise:

| Check | Reason |
|---|---|
| The system is of the registered class | `class` |
| The system has a row in `registry/systems.tsv` | `unknown` |
| The row's state is `live` | `state` |
| The row's `home` names this supervisor (a row with home `-` starts nowhere) | `not-home` |
| The store records no other live activation of this system | `active` |

The last two checks are the fence against a cloned store: a copy started under another home is refused, and a second
activation on the same home is refused. A whole machine cloned with both copies running passes both checks; that case
is bounded below, not excluded.

**Grant.** With `s` the clock second and `last` the stored `last_second`:

| Case | Result |
|---|---|
| First grant of this run, `s > last` (or no `last`) | Set `last_second = s` by compare-and-set; grant with index `offset`. |
| First grant of this run, `s = last` | Refuse `wait`: after any start the allocator waits for the next second. |
| `s < last` | Refuse `clock-behind`. |
| Later grant, `s` = the run's grant second | Grant with the next index; refuse `second-full` after 8,192. |
| Later grant, `s` above it | Set `last_second = s` by compare-and-set; grant with index `offset`. |
| The stored `last_second` changed under the run, or a compare-and-set fails | Refuse `contended`. |
| `s = 0`, or a term of 0 or above 24 hours | Refuse `clock-zero` or `term`. |

- A clock that ran ahead and granted leaves `last_second` in the future. The allocator then refuses until the clock
  passes it or the owner resets it. Refusing costs new leases, not uniqueness.
- There is no renewal and no lease table.

| System's store | What makes the compare-and-set atomic |
|---|---|
| Valkey | One server-side function on the single primary |
| An append-only journal | Check-and-append under the journal's file lock |
| SQLite | One row in one transaction |
| mxdb | One control record under the store's single writer |

## Restore and clone

- **A store restored from an old copy** cannot repeat a writer while the host clock at restart is later than the clock
  at the last grant the restore lost. Writer numbers come from the clock second, not from a counter in the store. The
  restored `last_second` is old, the clock is not, so the allocator grants from a later second. Leases lost with the
  newer state used earlier seconds and end within 24 hours, before any of those seconds' numbers comes round.
- **Two faults together defeat this:** a restored store and a clock set back to the lost grants' seconds. Against that
  case only the random index offset remains (a repeat needs the same index too), which is a defence, not a proof. An
  allocator whose memory is rolled back with its clock (a virtual machine resumed from a memory snapshot) also keeps
  its offset, and then repeats the lost grants exactly.
- **A cloned live minter** (a forked process or a cloned machine) repeats ids whenever both copies mint the same kind
  in the same millisecond, until each takes its next lease: at most 24 hours. The pid guard ([below](#pid-guard))
  closes the forked case. A system's uniqueness index catches what remains.

<a id="minter"></a>

## The minter

The same five lines in every language:

```text
t = max(clock_ms, last_t, not_before)        # never goes back; a slow clock stamps not_before
if t == last_t: count += 1 else: count = 0
if count == 2^18: t += 1; count = 0          # a burst borrows the next millisecond
if t > not_after: refuse "lease-ended"       # never stamps outside its window
last_t = t; emit (system, kind, t, writer, count)
```

- The count is per minter, across kinds: one minter never repeats a (time, count) pair.
- A refused mint changes no state.
- A kind that is not three letters is refused (`kind`).
- **Switching leases.** A minter past half its term asks for a fresh lease and switches to it. It keeps `last_t` and
  raises the new `not_before` to `last_t + 1`, so one minter's ids of a kind never go backward in byte order, even when
  the new writer number is lower. A lease of another system is refused (`lease-system`); a lease that ends before
  `last_t + 1` is refused (`lease-ended`).

<a id="pid-guard"></a>

### The pid guard

The minter records the process id it was created under and refuses to mint or switch under another (`fork`). A child
forked after the minter was created therefore cannot mint on its parent's lease; it takes its own lease.

<a id="detached"></a>

## Detached minting

A producer that can reach no allocator mints **detached**: it draws a detached system code (first character `3`–`7`)
and a random 30-bit writer from the operating system's random source, and uses the same minter with a window of at most
24 hours from its clock. A producer with a state directory keeps its code there, so its ids sort together; the writer
is drawn again at every process start.

- Two detached processes produce an equal id only if they share code and writer and also mint the same kind in the
  same millisecond at the same count. The chance that two given processes share code and writer is 2^-42.3; it is
  2^-30 for two copies of one state directory.
- If the random source fails, the minter returns an error. There is no fallback to a process id or a time.

<a id="door"></a>

## The door

A registered system admits incoming ids through its door. The door holds the system's code, the kinds it accepts, its
clock, a limit on how far ahead of that clock an id may be stamped (a parameter of the door; 5,000 ms by default), and
its own minter. It checks in this order:

| Check | Result |
|---|---|
| The id's kind is one the door accepts | otherwise refused, `kind` |
| The id is **detached** | never stored as the system's own fact. The door mints its own id of that kind and binds the detached id to it as a command token. Admitting the same detached id again returns the same bound id, so a retry does not create a second record. |
| The id's system is the door's system | otherwise refused, `foreign` |
| The id's time is at most `limit` milliseconds ahead of the door's clock | otherwise refused, `ahead` |
| When the session holds a lease from this system: the id's writer is the lease's writer and its time is inside the lease window | otherwise refused, `lease` |
| All of the above | admitted as the system's own id |

An id is a name, not a credential: admission still binds the authenticated principal, and a lease is a right to name,
never a right to write. A destination's fence stays a separate, checked number.

## Vectors

`vectors/lease.tsv` (lease texts and writer numbers) and `vectors/mint.tsv` (minter scenarios: the clock, the kind and
the expected text or refusal at each step, including bursts, a clock set back, a slow clock, the lease end and lease
switches).
