"""Every row of vectors/brd14.tsv at its layer, both ways; the order of embedded ids; seeded random brd14 ids."""

import sys
import unittest
from collections.abc import Callable

from eid import (
    BRD14_EPOCH,
    Eid,
    from_brd14,
    from_brd14_registered,
    parse,
    to_brd14,
    to_brd14_registered,
)

from .vectors import REGISTRY, outcome, report_checked, rows, unhex

BASE62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
REASONS = (
    "brd14-length",
    "brd14-namespace",
    "brd14-alphabet",
    "brd14-range",
    "brd14-system",
    "brd14-time",
    "brd14-writer",
    "brd14-count",
    "brd14-kind",
)


def brd_kinds(test: unittest.TestCase) -> set[str]:
    """The kinds that registry/kinds.tsv gives the system brd."""
    text = (REGISTRY / "kinds.tsv").read_text(encoding="utf-8")
    lines = [line for line in text.split("\n") if line != "" and not line.startswith("#")]
    test.assertEqual(lines[0], "system\tkind\trole\tstate\tserved_by\tmeaning")
    cells = [line.split("\t") for line in lines[1:]]
    return {c[1] for c in cells if c[0] == "brd"}


def answer(f: Callable[[], Eid | str]) -> str:
    """The text of what a call returns, or ``!reason`` when it refuses."""
    out: list[str] = []
    r = outcome(lambda: out.append(str(f())))
    return out[0] if r == "ok" else f"!{r}"


def cmp(a: object, b: object) -> int:
    return (a > b) - (a < b)  # type: ignore[operator]


def splitmix64(seed: int) -> Callable[[], int]:
    """splitmix64, seeded as the Go, Rust and TypeScript tests are."""
    mask = (1 << 64) - 1
    state = seed

    def next_() -> int:
        nonlocal state
        state = (state + 0x9E3779B97F4A7C15) & mask
        z = state
        z = ((z ^ (z >> 30)) * 0xBF58476D1CE4E5B9) & mask
        z = ((z ^ (z >> 27)) * 0x94D049BB133111EB) & mask
        return z ^ (z >> 31)

    return next_


class Brd14Test(unittest.TestCase):
    def test_every_row_at_its_layer_both_ways(self) -> None:
        data = rows("brd14.tsv")
        kinds = brd_kinds(self)
        checked = 0
        seen = set()
        # (case, op) that a registry row refuses, and (case, op) whose pure row accepts
        refused = {(r[0], r[1]) for r in data if r[2] == "registry"}
        accepted = set()
        for name, op, layer, input_, want in data:
            key = (name, op)
            if op == "from":
                b = unhex(input_)
                got = answer(lambda: from_brd14(b))
                got_registry = answer(lambda: from_brd14_registered(b, kinds))
            elif op == "to":
                x = parse(input_)
                got = answer(lambda: to_brd14(x))
                got_registry = answer(lambda: to_brd14_registered(x, kinds))
            else:
                self.fail(f"case {name}: op {op}")
            if layer == "pure":
                self.assertEqual((key, got), (key, want))
                if key not in refused:
                    self.assertEqual((key, got_registry), (key, want))
                if not want.startswith("!"):
                    accepted.add(key)
            elif layer == "registry":
                self.assertEqual((key, got_registry, key in accepted), (key, "!brd14-kind", True))
                self.assertEqual(want, "!brd14-kind")
            else:
                self.fail(f"case {name}: layer {layer}")
            if want.startswith("!"):
                seen.add(want[1:])
            checked += 1
        report_checked("brd14.tsv", checked, len(data))
        for r in REASONS:
            self.assertIn(r, seen)
        self.assertGreater(len(data), 0)
        self.assertEqual(checked, len(data))

    def test_byte_order_equals_brd14_order_and_random_ids_return(self) -> None:
        data = rows("brd14.tsv")
        pairs = [(r[4], parse(r[3])) for r in data if r[1] == "to" and r[2] == "pure" and not r[4].startswith("!")]
        self.assertGreater(len(pairs), 50)
        bad = sum(1 for a, x in pairs for b, y in pairs if cmp(a, b) != cmp(x, y))
        self.assertEqual(bad, 0)
        next_ = splitmix64(14)
        prev: tuple[str, Eid] | None = None
        n = 10000
        for _ in range(n):
            text = "".join(chr(65 + next_() % 26) for _ in range(3))
            v = next_() >> 1
            fields = (v >> 22, (v >> 12) & 1023, v & 4095)
            digits = []
            for _ in range(11):
                v, d = divmod(v, 62)
                digits.append(BASE62[d])
            text += "".join(reversed(digits))
            x = from_brd14(text)
            self.assertEqual((x.ms, x.writer, x.count), (BRD14_EPOCH + fields[0], fields[1], fields[2]), text)
            self.assertEqual(to_brd14(x), text)
            if prev is not None:
                self.assertEqual(cmp(prev[1], x), cmp(prev[0], text), text)
            prev = (text, x)
        print(
            f"\nBRD14 {len(pairs)} vector ids keep brd14 order pairwise; {n} random ids return and keep order",
            file=sys.stderr,
            flush=True,
        )

    def test_writer_is_the_node_and_count_is_the_seq(self) -> None:
        # node 1 and seq 2 at the epoch: the two fields are told apart.
        snow = (1 << 12) | 2
        digits = []
        for _ in range(11):
            snow, d = divmod(snow, 62)
            digits.append(BASE62[d])
        x = from_brd14("JOB" + "".join(reversed(digits)))
        self.assertEqual((x.system, x.kind, x.ms, x.writer, x.count), ("brd", "job", BRD14_EPOCH, 1, 2))
        self.assertEqual(outcome(lambda: from_brd14("job" + "".join(reversed(digits)))), "brd14-namespace")


if __name__ == "__main__":
    unittest.main()
