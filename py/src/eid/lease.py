"""The pure checks of a lease: its text form, its window and the writer number of a grant (spec/lease.md)."""

import re
from dataclasses import dataclass

from .id import TIME_MAX, WRITER_MAX, Code, _is_uint, code_class, code_to_string, parse_code
from .refusal import Refusal

LEASE_TERM_MAX = 86_400_000
"""The longest lease window, 24 hours in milliseconds."""
SECOND_SLOTS = 1 << 17
"""A writer number's second comes round after this many seconds (36.4 hours)."""
INDEX_SLOTS = 1 << 13
"""Grants per second per system."""

_DECIMAL = re.compile(r"0|[1-9][0-9]*")
_U64_MAX = (1 << 64) - 1


@dataclass(frozen=True, slots=True)
class Lease:
    """An exclusive right to one writer number and the id-times from ``not_before`` to ``not_after``, inclusive."""

    system: str
    writer: int
    not_before: int
    not_after: int


def _window(not_before: int, not_after: int) -> bool:
    return (
        not_before != 0
        and not_after >= not_before
        and not_after <= TIME_MAX
        and not_after - not_before <= LEASE_TERM_MAX
    )


def new_lease(system: str | Code, writer: int, not_before: int, not_after: int) -> Lease:
    """
    A lease, checked as the text form is checked. Raises a ``Refusal`` of reason ``lease-syntax`` (a system that is not
    three alphabet characters or a 15-bit code, a number that is not an ``int`` from 0 to 2^64 - 1), ``lease-system``,
    ``lease-writer`` or ``lease-window``.
    """
    code = parse_code(system) if isinstance(system, str) else system
    if code is None or not _is_uint(code, (1 << 15) - 1):
        raise Refusal("lease-syntax")
    for n in (writer, not_before, not_after):
        if not _is_uint(n, _U64_MAX):
            raise Refusal("lease-syntax")
    if code_class(code) == "reserved":
        raise Refusal("lease-system")
    if writer > WRITER_MAX:
        raise Refusal("lease-writer")
    if not _window(not_before, not_after):
        raise Refusal("lease-window")
    return Lease(code_to_string(code), writer, not_before, not_after)


def _lease_decimal(p: str) -> int:
    if len(p) > 20 or not _DECIMAL.fullmatch(p):
        raise Refusal("lease-syntax")
    v = int(p)
    if v > _U64_MAX:
        raise Refusal("lease-syntax")
    return v


def parse_lease(s: str) -> Lease:
    """
    The strict parse of ``system:writer:not_before:not_after``: a system of three alphabet characters and three decimal
    numbers below 2^64 without sign or leading zero. Raises a ``Refusal`` of reason ``lease-syntax``, ``lease-system``,
    ``lease-writer`` or ``lease-window``.
    """
    parts = s.split(":")
    if len(parts) != 4:
        raise Refusal("lease-syntax")
    system = parse_code(parts[0])
    if system is None:
        raise Refusal("lease-syntax")
    writer, not_before, not_after = (_lease_decimal(p) for p in parts[1:])
    if code_class(system) == "reserved":
        raise Refusal("lease-system")
    if writer > WRITER_MAX:
        raise Refusal("lease-writer")
    if not _window(not_before, not_after):
        raise Refusal("lease-window")
    return Lease(code_to_string(system), writer, not_before, not_after)


def format_lease(lease: Lease) -> str:
    """The text form of a lease."""
    return f"{lease.system}:{lease.writer}:{lease.not_before}:{lease.not_after}"


def writer_for(second: int, index: int) -> int:
    """
    ``(second mod 2^17) * 2^13 + index mod 2^13``: the writer number of the grant of an allocator. Raises a ``Refusal``
    of reason ``range`` for an argument that is not a non-negative ``int``.
    """
    if not isinstance(second, int) or isinstance(second, bool) or second < 0:
        raise Refusal("range")
    if not isinstance(index, int) or isinstance(index, bool) or index < 0:
        raise Refusal("range")
    return (second % SECOND_SLOTS) * INDEX_SLOTS + (index % INDEX_SLOTS)
