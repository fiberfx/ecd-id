"""Reading the language-neutral vectors of vectors/ and reporting how many rows a test checked."""

import re
import sys
from collections.abc import Callable
from pathlib import Path

from eid import Refusal

ROOT = Path(__file__).resolve().parents[2]
VECTORS = ROOT / "vectors"
REGISTRY = ROOT / "registry"

_HEX = re.compile(r"(?:[0-9a-f]{2})*")


def rows(name: str) -> list[list[str]]:
    """The data rows of a vector file: comment lines and empty lines dropped, cells split on tabs."""
    text = (VECTORS / name).read_text(encoding="utf-8")
    return [line.split("\t") for line in text.split("\n") if line != "" and not line.startswith("#")]


def unhex(s: str) -> bytes:
    """Lowercase hex to bytes; ``-`` is the empty input."""
    if s == "-":
        return b""
    if not _HEX.fullmatch(s):
        raise ValueError(f"bad hex {s}")
    return bytes.fromhex(s)


def outcome(f: Callable[[], object]) -> str:
    """The refusal reason of a call, or ``ok`` when it returns."""
    try:
        f()
    except Refusal as e:
        return e.reason
    return "ok"


def report_checked(file: str, checked: int, total: int) -> None:
    """
    Prints ``VECTORS <file> <checked>/<rows>`` to standard error for the rows script. The line starts after a newline,
    because the verbose runner has written the test's name without one.
    """
    print(f"\nVECTORS {file} {checked}/{total}", file=sys.stderr, flush=True)
