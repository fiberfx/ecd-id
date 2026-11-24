"""The Ecd ID: the 16-byte layout, the alphabet, the canonical text and its strict parse (spec/id.md)."""

from collections.abc import Buffer
from typing import Literal, cast

from .refusal import Refusal

ALPHABET = "234567abcdefghijklmnopqrstuvwxyz"
"""The 32 symbols in value order; ASCII order equals value order."""

TEXT_LENGTH = 28
"""Length of the canonical text."""
BYTE_LENGTH = 16
"""Length of the binary form."""
TIME_MAX = 2**48 - 1
"""The largest millisecond, 2^48 - 1."""
WRITER_MAX = 2**30 - 1
"""The largest writer number, 2^30 - 1."""
COUNT_LIMIT = 2**18
"""One more than the largest count, 2^18."""

Code = int
"""A 15-bit code of three alphabet characters: a system code or a kind."""

SystemClass = Literal["registered", "detached", "reserved"]
"""The class of a system code, read from its first character."""

_NOT_IN_ALPHABET = 0xFF


def _table(symbols: str) -> bytes:
    """A table from a byte to its value among the symbols; a byte outside them maps to 0xFF."""
    t = bytearray([_NOT_IN_ALPHABET]) * 256
    for value, c in enumerate(symbols.encode("ascii")):
        t[c] = value
    return bytes(t)


_VALUE = _table(ALPHABET)


def _text_bytes(text: str | Buffer) -> bytes:
    """The bytes of a text: a ``str`` as its UTF-8 bytes, a buffer as it is."""
    if isinstance(text, str):
        # A lone surrogate becomes three bytes outside the alphabet, as U+FFFD does in other implementations.
        return text.encode("utf-8", "surrogatepass")
    if isinstance(text, Buffer):
        return bytes(text)
    raise TypeError(f"expected str or a bytes-like object, not {type(text).__name__}")


def _is_uint(v: object, limit: int) -> bool:
    """Whether ``v`` is an ``int`` (not a ``bool``) from 0 to ``limit``."""
    return isinstance(v, int) and not isinstance(v, bool) and 0 <= v <= limit


def parse_code(s: str) -> Code | None:
    """Parses three alphabet characters; uppercase and any other character give ``None``."""
    if not isinstance(s, str) or len(s) != 3:
        return None
    v = 0
    for ch in s:
        c = ord(ch)
        x = _VALUE[c] if c < 256 else _NOT_IN_ALPHABET
        if x == _NOT_IN_ALPHABET:
            return None
        v = (v << 5) | x
    return v


def code_to_string(c: Code) -> str:
    """The three characters of a code."""
    return ALPHABET[(c >> 10) & 31] + ALPHABET[(c >> 5) & 31] + ALPHABET[c & 31]


def code_class(c: Code) -> SystemClass:
    """The class of a code read as a system code: ``2`` is reserved, ``3`` to ``7`` detached, a letter registered."""
    first = (c >> 10) & 31
    if first == 0:
        return "reserved"
    if first <= 5:
        return "detached"
    return "registered"


def is_kind(c: Code) -> bool:
    """Whether a code is a valid kind: three letters."""
    return ((c >> 10) & 31) >= 6 and ((c >> 5) & 31) >= 6 and (c & 31) >= 6


SYS: Code = cast(Code, parse_code("sys"))
"""The kind that names a system itself, in every system."""


def pack(system: Code, kind: Code, ms: int, writer: int, count: int) -> bytes:
    """Packs checked fields into 16 bytes, big-endian: system, kind, two zero bits, time, writer and count."""
    v = (system << 113) | (kind << 98) | (ms << 48) | (writer << 18) | count
    return v.to_bytes(BYTE_LENGTH, "big")


def _decode_text(t: bytes) -> bytes:
    """Steps 1 to 4 of the strict parse; returns the 16 bytes, not yet checked for steps 5 to 7."""
    if len(t) != TEXT_LENGTH:
        raise Refusal("length")
    if t[3] != 0x5F or t[7] != 0x5F:
        raise Refusal("separator")
    acc = 0
    for pos in range(TEXT_LENGTH):
        if pos == 3 or pos == 7:
            continue
        v = _VALUE[t[pos]]
        if v == _NOT_IN_ALPHABET:
            raise Refusal("alphabet")
        acc = (acc << 5) | v
    # 26 symbols carry 130 bits: the 128 bits of the id, then two pad bits.
    if acc & 3:  # pad check
        raise Refusal("pad")
    return (acc >> 2).to_bytes(BYTE_LENGTH, "big")


def _encode_text(b: bytes) -> str:
    """Encodes 16 bytes as the 28-character canonical text."""
    v = int.from_bytes(b, "big") << 2
    s = "".join(ALPHABET[(v >> (5 * (25 - i))) & 31] for i in range(26))
    return s[:3] + "_" + s[3:6] + "_" + s[6:]


class Eid:
    """
    An Ecd ID: 16 bytes, big-endian. The comparison operators compare the bytes, which compares system, kind, time,
    writer and count. An ``Eid`` is immutable and hashable; ``str()`` gives the canonical text.

    Constructing one is binary ingress: exactly 16 bytes, then steps 5 to 7 of the strict parse. The constructor raises
    a ``Refusal`` of reason ``length``, ``reserved``, ``system-reserved`` or ``kind``, and ``TypeError`` for an input
    that is not bytes-like.
    """

    __slots__ = ("_b",)
    _b: bytes

    def __init__(self, data: Buffer) -> None:
        if not isinstance(data, Buffer):
            raise TypeError(f"expected a bytes-like object, not {type(data).__name__}")
        b = bytes(data)
        if len(b) != BYTE_LENGTH:
            raise Refusal("length")
        if b[3] & 3:
            raise Refusal("reserved")
        if code_class((b[0] << 7) | (b[1] >> 1)) == "reserved":
            raise Refusal("system-reserved")
        if not is_kind(((b[1] & 1) << 14) | (b[2] << 6) | (b[3] >> 2)):
            raise Refusal("kind")
        object.__setattr__(self, "_b", b)

    def __setattr__(self, name: str, value: object) -> None:
        raise AttributeError("an Eid is immutable")

    def __delattr__(self, name: str) -> None:
        raise AttributeError("an Eid is immutable")

    def __reduce__(self) -> tuple[type["Eid"], tuple[bytes]]:
        return (Eid, (self._b,))

    @property
    def system_code(self) -> Code:
        """The system's 15-bit code."""
        return ((self._b[0] << 7) | (self._b[1] >> 1)) & 0x7FFF

    @property
    def kind_code(self) -> Code:
        """The kind's 15-bit code."""
        return (((self._b[1] & 1) << 14) | (self._b[2] << 6) | (self._b[3] >> 2)) & 0x7FFF

    @property
    def system(self) -> str:
        """The system that minted the id: three characters."""
        return code_to_string(self.system_code)

    @property
    def kind(self) -> str:
        """The kind: three letters."""
        return code_to_string(self.kind_code)

    @property
    def reserved(self) -> int:
        """The two reserved bits; zero in every valid id."""
        return self._b[3] & 3

    @property
    def ms(self) -> int:
        """Unix milliseconds stamped by the minter, below 2^48."""
        return int.from_bytes(self._b[4:10], "big")

    @property
    def writer(self) -> int:
        """The writer number, below 2^30."""
        return int.from_bytes(self._b[10:16], "big") >> 18

    @property
    def count(self) -> int:
        """The count inside the writer's millisecond, below 2^18."""
        return int.from_bytes(self._b[10:16], "big") & (COUNT_LIMIT - 1)

    @property
    def class_(self) -> SystemClass:
        """The class of the id's system: ``registered`` or ``detached``."""
        return code_class(self.system_code)

    def __str__(self) -> str:
        """The canonical text."""
        return _encode_text(self._b)

    def __repr__(self) -> str:
        return f"eid.parse({_encode_text(self._b)!r})"

    def __bytes__(self) -> bytes:
        return self._b

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Eid):
            return NotImplemented
        return self._b == other._b

    def __lt__(self, other: "Eid") -> bool:
        if not isinstance(other, Eid):
            return NotImplemented
        return self._b < other._b

    def __le__(self, other: "Eid") -> bool:
        if not isinstance(other, Eid):
            return NotImplemented
        return self._b <= other._b

    def __gt__(self, other: "Eid") -> bool:
        if not isinstance(other, Eid):
            return NotImplemented
        return self._b > other._b

    def __ge__(self, other: "Eid") -> bool:
        if not isinstance(other, Eid):
            return NotImplemented
        return self._b >= other._b

    def __hash__(self) -> int:
        return hash(self._b)

    # Last in the class body, so that no annotation above reads the property in place of the built-in type.
    @property
    def bytes(self) -> bytes:
        """The 16 bytes."""
        return self._b


def parse(text: str | Buffer) -> Eid:
    """
    The strict parse of the canonical text (spec/id.md#strict-parse). A ``str`` is read as its UTF-8 bytes, so a
    non-ASCII character fails as the specification says; a bytes-like input is read as it is. Raises a ``Refusal`` of
    reason ``length``, ``separator``, ``alphabet``, ``pad``, ``reserved``, ``system-reserved`` or ``kind``.
    """
    return Eid(_decode_text(_text_bytes(text)))


def from_bytes(data: Buffer) -> Eid:
    """Binary ingress: exactly 16 bytes, then steps 5 to 7 of the strict parse. Raises a ``Refusal``."""
    return Eid(data)


def format(eid: Eid | Buffer) -> str:
    """The canonical text of an id, or of 16 bytes that pass binary ingress. Raises a ``Refusal`` for refused bytes."""
    return str(eid if isinstance(eid, Eid) else Eid(eid))


def valid(text: str | Buffer) -> bool:
    """Whether the input is a valid canonical text."""
    try:
        parse(text)
    except Refusal:
        return False
    return True


def valid_binary(data: Buffer) -> bool:
    """Whether the 16 bytes pass binary ingress."""
    try:
        from_bytes(data)
    except Refusal:
        return False
    return True


def _code_of(v: str | Code, on_bad: Literal["alphabet", "kind"]) -> Code:
    if isinstance(v, str):
        c = parse_code(v)
        if c is None:
            raise Refusal(on_bad)
        return c
    if not _is_uint(v, (1 << 15) - 1):
        raise Refusal("range")
    return v


def from_parts(system: str | Code, kind: str | Code, ms: int, writer: int, count: int) -> Eid:
    """
    Builds an id from its fields, checking each against the layout. ``system`` and ``kind`` are three characters or a
    15-bit code. Raises a ``Refusal`` of reason ``system-reserved``, ``kind`` or ``range`` (a code, time, writer or
    count that is not an ``int`` in its field's range); a system given as a string that is not three alphabet
    characters is refused with ``alphabet``.
    """
    s = _code_of(system, "alphabet")
    if code_class(s) == "reserved":
        raise Refusal("system-reserved")
    k = _code_of(kind, "kind")
    if not is_kind(k):
        raise Refusal("kind")
    if not _is_uint(ms, TIME_MAX) or not _is_uint(writer, WRITER_MAX) or not _is_uint(count, COUNT_LIMIT - 1):
        raise Refusal("range")
    return Eid(pack(s, k, ms, writer, count))


def system_id(system: str | Code) -> Eid:
    """The system's own id: its code, kind ``sys`` and a zero local part."""
    return from_parts(system, SYS, 0, 0, 0)


def compare(a: Eid, b: Eid) -> int:
    """Compares two ids in byte order: -1, 0 or 1."""
    return (a.bytes > b.bytes) - (a.bytes < b.bytes)
