# brd14 ids under the system `brd`

Every brd14 id maps to exactly one Ecd ID of the system `brd`, without loss and in the same order. Existing ids are
kept, not re-minted: a document keeps its 14-character id, and a store that wants 16 bytes uses the embedding.

## brd14, as read here

A brd14 id is 14 bytes: a namespace of three letters `A`–`Z`, then 11 Base62 digits (`0-9A-Za-z`, ASCII order) of a
63-bit number laid out as `ts(41) << 22 | node(10) << 12 | seq(12)`, with epoch 1704067200000 ms (2024-01-01T00:00:00Z).
Decoding refuses in brd14's own order and with its own classes:

| Step | Check | Reason |
|---|---|---|
| 1 | Exactly 14 bytes | `brd14-length` |
| 2 | Bytes 0–2 are `A`–`Z` | `brd14-namespace` |
| 3 | Bytes 3–13 are Base62 digits | `brd14-charset` |
| 4 | The payload is at most `AzL8n0Y58m7` (2^63 − 1) | `brd14-range` |

A text refused by any step is refused before embedding.

## The map

| Ecd field | From the brd14 id |
|---|---|
| system | `brd` |
| kind | the namespace in lowercase |
| time | `ts + 1704067200000` |
| writer | `node` |
| count | `seq` |

```text
KAN0SHfNmk4iYR  →  brd_kan_23oc3tmlqq2222ti222w
EVT000xY9Wvvcd  →  brd_evt_23ln4ott622222gf2eew
```

- **Inverse.** An Ecd ID of system `brd` whose time is in `[epoch, epoch + 2^41)`, whose writer is below 1024 and
  whose count is below 4096 maps back to its brd14 text; any other id is refused (`not-embedding`).
- **Order.** brd14 text order is (namespace, number); uppercase letters keep their relative order as lowercase
  letters in this alphabet, and the number's fields are already time, node, sequence. Sorting brd14 texts and sorting
  their embeddings give the same order.
- **Arithmetic only.** The map reads the number, not a profile, so it also holds for an id minted under another brd14
  layout profile; only the wall-clock reading of its time field is then off.
- **No repair.** Two equal brd14 ids stay equal.
- **Letter case.** brd14 ids that differ only by letter case have different numbers, so their embeddings differ, and
  each embedding is one-case text.
- **Membership.** The map does not consult the registry: any three-letter namespace embeds.

## Vectors

- `vectors/embed_brd14.tsv`: brd14's frozen valid vectors (`model/fixtures/brd14-valid.tsv`, copied unchanged) with
  their embeddings.
- `vectors/embed_invalid.tsv`: brd14's frozen invalid vectors, each refused with brd14's class.
- `vectors/embed_kan.tsv`: a corpus of document ids in the `KAN` namespace (`model/fixtures/kan-ids.txt`), kept where
  they decode as valid brd14, with their embeddings. The file's header states the count and how many groups differ
  only by letter case.
