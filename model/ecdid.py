"""Reference model of the Ecd ID, version 1 (Python standard library only).

This module is the oracle: model/vectors.py uses it to generate every file in vectors/, and the Rust and Go
implementations must agree with those files byte for byte. It favours plain arithmetic over speed.
"""

ALPHABET = "234567abcdefghijklmnopqrstuvwxyz"
VALUE = {c: i for i, c in enumerate(ALPHABET)}
TEXT_LEN = 28
TIME_MAX = (1 << 48) - 1
WRITER_MAX = (1 << 30) - 1
COUNT_LIMIT = 1 << 18
LEASE_TERM_MAX = 86_400_000
SECOND_SLOTS = 1 << 17
INDEX_SLOTS = 1 << 13
BRD14_EPOCH = 1_704_067_200_000
BRD14_ALPHABET = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
BRD14_MAX_PAYLOAD = "AzL8n0Y58m7"
BRD = None  # set below, the code of the system that holds brd14 embeddings


class Refused(Exception):
    def __init__(self, reason):
        super().__init__(reason)
        self.reason = reason


def code(s):
    """Three alphabet characters to a 15-bit number."""
    if len(s) != 3 or any(c not in VALUE for c in s):
        raise Refused("code")
    return (VALUE[s[0]] << 10) | (VALUE[s[1]] << 5) | VALUE[s[2]]


def code_text(v):
    return ALPHABET[(v >> 10) & 31] + ALPHABET[(v >> 5) & 31] + ALPHABET[v & 31]


BRD = code("brd")
SYS = code("sys")


def system_class(system):
    first = system >> 10
    if first == 0:
        return "reserved"
    if first <= 5:
        return "detached"
    return "registered"


def kind_valid(kind):
    return all(((kind >> s) & 31) >= 6 for s in (10, 5, 0))


def pack(system, kind, time, writer, count, reserved=0):
    assert 0 <= system < 1 << 15 and 0 <= kind < 1 << 15 and 0 <= reserved < 4
    assert 0 <= time <= TIME_MAX and 0 <= writer <= WRITER_MAX and 0 <= count < COUNT_LIMIT
    n = (system << 113) | (kind << 98) | (reserved << 96) | (time << 48) | (writer << 18) | count
    return n.to_bytes(16, "big")


def fields(b):
    n = int.from_bytes(b, "big")
    return {
        "system": n >> 113,
        "kind": (n >> 98) & 0x7FFF,
        "reserved": (n >> 96) & 3,
        "time": (n >> 48) & TIME_MAX,
        "writer": (n >> 18) & WRITER_MAX,
        "count": n & (COUNT_LIMIT - 1),
    }


def encode(b):
    n = int.from_bytes(b, "big") << 2
    s = "".join(ALPHABET[(n >> (5 * (25 - i))) & 31] for i in range(26))
    return s[:3] + "_" + s[3:6] + "_" + s[6:]


def check_binary(b):
    """Binary ingress: the steps of the strict parse that apply to 16 bytes."""
    if len(b) != 16:
        raise Refused("length")
    f = fields(b)
    if f["reserved"]:
        raise Refused("reserved")
    if system_class(f["system"]) == "reserved":
        raise Refused("system-reserved")
    if not kind_valid(f["kind"]):
        raise Refused("kind")
    return bytes(b)


def parse(t):
    """Strict parse of the canonical text, given as bytes. Returns 16 bytes or raises Refused."""
    if isinstance(t, str):
        t = t.encode("utf-8")
    if len(t) != TEXT_LEN:
        raise Refused("length")
    if t[3] != 0x5F or t[7] != 0x5F:
        raise Refused("separator")
    s = t[:3] + t[4:7] + t[8:]
    n = 0
    for c in s:
        ch = chr(c)
        if ch not in VALUE:
            raise Refused("alphabet")
        n = (n << 5) | VALUE[ch]
    if n & 3:
        raise Refused("pad")
    b = (n >> 2).to_bytes(16, "big")
    check_binary(b)
    assert encode(b).encode() == t
    return b


# ---- the Valkey stream-id mapping

def stream_id(b):
    f = fields(b)
    return f["time"], (f["system"] << 48) | (f["writer"] << 18) | f["count"]


def stream_text(b):
    ms, seq = stream_id(b)
    return f"{ms}-{seq}"


def legacy_tail(b):
    """The 48-bit tail without the system (bytes 10-15). Kept only as the control for collisions."""
    f = fields(b)
    return (f["writer"] << 18) | f["count"]


def parse_stream_text(s):
    parts = s.split("-")
    if len(parts) != 2 or not all(p and p.isascii() and p.isdigit() for p in parts):
        raise Refused("stream-syntax")
    if any(len(p) > 1 and p[0] == "0" for p in parts):
        raise Refused("stream-syntax")
    ms, seq = int(parts[0]), int(parts[1])
    if ms >= 1 << 64 or seq >= 1 << 64:
        raise Refused("stream-syntax")
    return ms, seq


def from_stream(kind_text, ms, seq):
    """Inverse of stream_id, given the kind that the stream carries."""
    try:
        kind = code(kind_text)
    except Refused:
        raise Refused("kind")
    if not kind_valid(kind):
        raise Refused("kind")
    if ms > TIME_MAX or seq >= 1 << 63:
        raise Refused("stream-range")
    system = seq >> 48
    if system_class(system) == "reserved":
        raise Refused("system-reserved")
    return pack(system, kind, ms, (seq >> 18) & WRITER_MAX, seq & (COUNT_LIMIT - 1))


def valkey_accepts(top, entry):
    """Valkey's rule for XADD with an explicit id: the id must be greater than the stream's top, and not 0-0."""
    if entry == (0, 0):
        return False
    return top is None or entry > top


# ---- the system's own id (the mxdb owner)

def system_id(system_text):
    system = code(system_text)
    if system_class(system) == "reserved":
        raise Refused("system-reserved")
    return pack(system, SYS, 0, 0, 0)


# ---- leases

def writer_for(second, index):
    assert 0 <= index < INDEX_SLOTS
    return ((second % SECOND_SLOTS) << 13) | index


def _decimal(p):
    if not p or not p.isascii() or not p.isdigit() or (len(p) > 1 and p[0] == "0") or len(p) > 20:
        raise Refused("lease-syntax")
    if int(p) >= 1 << 64:
        raise Refused("lease-syntax")
    return int(p)


def parse_lease(s):
    parts = s.split(":")
    if len(parts) != 4:
        raise Refused("lease-syntax")
    try:
        system = code(parts[0])
    except Refused:
        raise Refused("lease-syntax")
    writer, nb, na = (_decimal(p) for p in parts[1:])
    if system_class(system) == "reserved":
        raise Refused("lease-system")
    if writer > WRITER_MAX:
        raise Refused("lease-writer")
    if nb == 0 or na < nb or na > TIME_MAX or na - nb > LEASE_TERM_MAX:
        raise Refused("lease-window")
    return (system, writer, nb, na)


def lease_text(lease):
    system, writer, nb, na = lease
    return f"{code_text(system)}:{writer}:{nb}:{na}"


class Minter:
    """The minter of section 'The minter' in spec/lease.md."""

    def __init__(self, lease):
        self.system, self.writer, self.not_before, self.not_after = lease
        self.last_t = None
        self.count = 0

    def mint(self, clock_ms, kind_text):
        kind = code(kind_text) if len(kind_text) == 3 and all(c in VALUE for c in kind_text) else None
        if kind is None or not kind_valid(kind):
            raise Refused("kind")
        last = -1 if self.last_t is None else self.last_t
        t = max(clock_ms, last, self.not_before)
        count = self.count + 1 if t == self.last_t else 0
        if count == COUNT_LIMIT:
            t, count = t + 1, 0
        if t > self.not_after:
            raise Refused("lease-ended")
        self.last_t, self.count = t, count
        return pack(self.system, kind, t, self.writer, count)

    def switch(self, lease):
        system, writer, nb, na = lease
        if system != self.system:
            raise Refused("lease-system")
        if self.last_t is not None:
            nb = max(nb, self.last_t + 1)
        if nb > na:
            raise Refused("lease-ended")
        self.writer, self.not_before, self.not_after = writer, nb, na


# ---- brd14 embedding under the system brd

def brd14_decode(t):
    if isinstance(t, str):
        t = t.encode("utf-8")
    if len(t) != 14:
        raise Refused("brd14-length")
    if any(not (0x41 <= c <= 0x5A) for c in t[:3]):
        raise Refused("brd14-namespace")
    payload = t[3:].decode("latin-1")
    if any(c not in BRD14_ALPHABET for c in payload):
        raise Refused("brd14-charset")
    if payload > BRD14_MAX_PAYLOAD:
        raise Refused("brd14-range")
    snow = 0
    for c in payload:
        snow = snow * 62 + BRD14_ALPHABET.index(c)
    return t[:3].decode(), snow


def brd14_encode(ns, snow):
    digits = []
    for _ in range(11):
        snow, r = divmod(snow, 62)
        digits.append(BRD14_ALPHABET[r])
    return ns + "".join(reversed(digits))


def embed(t):
    ns, snow = brd14_decode(t)
    return pack(BRD, code(ns.lower()), (snow >> 22) + BRD14_EPOCH, (snow >> 12) & 1023, snow & 4095)


def unembed(b):
    f = fields(b)
    t = f["time"] - BRD14_EPOCH
    if f["system"] != BRD or not 0 <= t < 1 << 41 or f["writer"] >= 1024 or f["count"] >= 4096:
        raise Refused("not-embedding")
    return brd14_encode(code_text(f["kind"]).upper(), (t << 22) | (f["writer"] << 12) | f["count"])
