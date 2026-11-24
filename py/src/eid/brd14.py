"""The embedding of brd14 ids into ids of the system ``brd``, both ways (spec/registry.md#brd14)."""

from collections.abc import Buffer, Container
from typing import cast

from .id import Code, Eid, _table, _text_bytes, from_parts, parse_code
from .refusal import Refusal

BRD14_LENGTH = 14
"""The length of a brd14 id in bytes: a namespace of three bytes ``A``-``Z``, then 11 Base62 digits."""
BRD14_EPOCH = 1704067200000
"""The brd14 epoch, 2024-01-01T00:00:00Z, in Unix milliseconds."""

_BASE62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
_NOT_BASE62 = 0xFF
_DIGIT = _table(_BASE62)
_BRD: Code = cast(Code, parse_code("brd"))


def from_brd14(text: str | Buffer) -> Eid:
    """
    Embeds a brd14 id into an id of the system ``brd``: the kind is the namespace in lowercase, the time is
    ``BRD14_EPOCH`` plus the snowflake's milliseconds, the writer its node and the count its sequence. A ``str`` is read
    as its UTF-8 bytes, a bytes-like input as it is. It reads no registry. Raises a ``Refusal`` of reason
    ``brd14-length``, ``brd14-namespace``, ``brd14-alphabet`` or ``brd14-range``, checked in the order of brd14's own
    decode.
    """
    b = _text_bytes(text)
    if len(b) != BRD14_LENGTH:
        raise Refusal("brd14-length")
    for c in b[:3]:
        if not 0x41 <= c <= 0x5A:
            raise Refusal("brd14-namespace")
    snow = 0
    for c in b[3:]:
        d = _DIGIT[c]
        if d == _NOT_BASE62:
            raise Refusal("brd14-alphabet")
        snow = snow * 62 + d
    if snow >= 1 << 63:
        raise Refusal("brd14-range")
    kind = cast(Code, parse_code(b[:3].decode("ascii").lower()))
    ms = BRD14_EPOCH + (snow >> 22)
    node = (snow >> 12) & 1023
    seq = snow & 4095
    return from_parts(system=_BRD, kind=kind, ms=ms, writer=node, count=seq)  # brd14 parts


def to_brd14(eid: Eid) -> str:
    """
    The inverse of ``from_brd14``. It reads no registry. Raises a ``Refusal`` of reason ``brd14-system``,
    ``brd14-time``, ``brd14-writer`` or ``brd14-count``.
    """
    if eid.system_code != _BRD:
        raise Refusal("brd14-system")
    ts = eid.ms - BRD14_EPOCH
    if ts < 0 or ts >= 1 << 41:
        raise Refusal("brd14-time")
    if eid.writer >= 1 << 10:
        raise Refusal("brd14-writer")
    if eid.count >= 1 << 12:
        raise Refusal("brd14-count")
    snow = (ts << 22) | (eid.writer << 12) | eid.count
    digits = []
    for _ in range(11):
        snow, d = divmod(snow, 62)
        digits.append(_BASE62[d])
    return eid.kind.upper() + "".join(reversed(digits))


def from_brd14_registered(text: str | Buffer, brd_kinds: Container[str]) -> Eid:
    """
    The registry layer of ``from_brd14``. The package reads no registry: ``brd_kinds`` holds the kinds that
    registry/kinds.tsv gives the system ``brd``, in any state, and any other kind is refused with ``brd14-kind``.
    """
    eid = from_brd14(text)
    if eid.kind not in brd_kinds:
        raise Refusal("brd14-kind")
    return eid


def to_brd14_registered(eid: Eid, brd_kinds: Container[str]) -> str:
    """The registry layer of ``to_brd14``, with ``brd_kinds`` as for ``from_brd14_registered``."""
    text = to_brd14(eid)
    if eid.kind not in brd_kinds:
        raise Refusal("brd14-kind")
    return text
