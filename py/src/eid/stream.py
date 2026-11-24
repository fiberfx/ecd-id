"""The Valkey stream-id mapping of an id and its inverse (spec/stream.md)."""

import re
from typing import NamedTuple

from .id import COUNT_LIMIT, TIME_MAX, WRITER_MAX, Eid, code_class, is_kind, pack, parse_code
from .refusal import Refusal


class StreamId(NamedTuple):
    """A Valkey stream entry id: two unsigned 64-bit numbers. Tuple order is stream order: ``ms``, then ``seq``."""

    ms: int
    seq: int


_U64 = 1 << 64
_SEQ_LIMIT = 1 << 63
_DECIMAL = re.compile(r"0|[1-9][0-9]*")


def format_stream_id(s: StreamId) -> str:
    """The text ``<ms>-<seq>``."""
    return f"{s.ms}-{s.seq}"


def stream_less(a: StreamId, b: StreamId) -> bool:
    """Whether ``a`` orders before ``b``."""
    return a.ms < b.ms or (a.ms == b.ms and a.seq < b.seq)


def _stream_decimal(p: str) -> int:
    # 2^64 has 20 digits; the length bound also keeps int() inside its limit on digits.
    if len(p) > 20 or not _DECIMAL.fullmatch(p):
        raise Refusal("stream-syntax")
    v = int(p)
    if v >= _U64:
        raise Refusal("stream-syntax")
    return v


def parse_stream_id(s: str) -> StreamId:
    """
    Parses ``<ms>-<seq>``: two decimal numbers below 2^64, without sign or leading zero, joined by one ``-``. Raises a
    ``Refusal`` of reason ``stream-syntax``.
    """
    cut = s.find("-")
    if cut < 0:
        raise Refusal("stream-syntax")
    return StreamId(_stream_decimal(s[:cut]), _stream_decimal(s[cut + 1 :]))


def to_stream_id(eid: Eid) -> StreamId:
    """The stream entry id of an id: time, then ``system << 48 | writer << 18 | count``."""
    return StreamId(eid.ms, (eid.system_code << 48) | legacy_tail(eid))


def legacy_tail(eid: Eid) -> int:
    """
    The 48-bit tail without the system (bytes 10 to 15). It is not unique across systems and exists only as the control
    that shows why the stream id carries the system.
    """
    return eid.writer * COUNT_LIMIT + eid.count


def from_stream_id(kind: str, sid: StreamId) -> Eid:
    """
    Rebuilds an id from a stream entry id and the kind the stream carries. Raises a ``Refusal`` of reason ``kind``,
    ``stream-range`` (``ms`` at least 2^48 or ``seq`` at least 2^63, or either negative) or ``system-reserved``.
    """
    k = parse_code(kind)
    if k is None or not is_kind(k):
        raise Refusal("kind")
    if not 0 <= sid.ms <= TIME_MAX or not 0 <= sid.seq < _SEQ_LIMIT:
        raise Refusal("stream-range")
    system = sid.seq >> 48
    if code_class(system) == "reserved":
        raise Refusal("system-reserved")
    writer = (sid.seq >> 18) & WRITER_MAX
    count = sid.seq & (COUNT_LIMIT - 1)
    return Eid(pack(system, k, sid.ms, writer, count))


def valkey_accepts(top: StreamId | None, entry: StreamId) -> bool:
    """
    Valkey's rule for XADD with an explicit id: the id is greater than the stream's top entry and is not 0-0. ``top`` is
    ``None`` for an empty stream.
    """
    if entry.ms == 0 and entry.seq == 0:
        return False
    return top is None or stream_less(top, entry)
