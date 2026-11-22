package eid

import (
	"strconv"
	"strings"
)

// StreamID is a Valkey stream entry id: two unsigned 64-bit numbers, ordered by Ms, then Seq.
type StreamID struct {
	Ms  uint64
	Seq uint64
}

func (s StreamID) String() string {
	return strconv.FormatUint(s.Ms, 10) + "-" + strconv.FormatUint(s.Seq, 10)
}

// Less reports whether s orders before o.
func (s StreamID) Less(o StreamID) bool {
	return s.Ms < o.Ms || (s.Ms == o.Ms && s.Seq < o.Seq)
}

func streamDecimal(p string) (uint64, error) {
	if p == "" || (len(p) > 1 && p[0] == '0') {
		return 0, ErrStreamSyntax
	}
	for i := 0; i < len(p); i++ {
		if p[i] < '0' || p[i] > '9' {
			return 0, ErrStreamSyntax
		}
	}
	v, err := strconv.ParseUint(p, 10, 64)
	if err != nil {
		return 0, ErrStreamSyntax
	}
	return v, nil
}

// ParseStreamID parses <ms>-<seq>: two decimal numbers below 2^64, without sign or leading zero, joined by one -.
func ParseStreamID(s string) (StreamID, error) {
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return StreamID{}, ErrStreamSyntax
	}
	ms, err := streamDecimal(a)
	if err != nil {
		return StreamID{}, err
	}
	seq, err := streamDecimal(b)
	if err != nil {
		return StreamID{}, err
	}
	return StreamID{ms, seq}, nil
}

// StreamID returns the stream entry id: time, then system<<48 | writer<<18 | count.
func (id ID) StreamID() StreamID {
	return StreamID{id.Time(), uint64(id.System())<<48 | uint64(id.Writer())<<18 | uint64(id.Count())}
}

// LegacyTail returns the 48-bit tail without the system (bytes 10-15). It is not unique across systems; it exists
// only as the control that shows why the stream id carries the system.
func (id ID) LegacyTail() uint64 {
	return uint64(id.Writer())<<18 | uint64(id.Count())
}

// FromStream rebuilds an id from a stream entry id and the kind that the stream carries.
func FromStream(kind string, sid StreamID) (ID, error) {
	k, ok := ParseCode(kind)
	if !ok || !k.IsKind() {
		return ID{}, ErrKind
	}
	if sid.Ms > TimeMax || sid.Seq >= 1<<63 {
		return ID{}, ErrStreamRange
	}
	system := Code(sid.Seq >> 48)
	if system.Class() == Reserved {
		return ID{}, ErrSystemReserved
	}
	return pack(system, k, sid.Ms, uint32(sid.Seq>>18)&WriterMax, uint32(sid.Seq)&(CountLimit-1)), nil
}

// ValkeyAccepts is Valkey's rule for XADD with an explicit id: the id is greater than the stream's top entry and is
// not 0-0. hasTop is false for an empty stream.
func ValkeyAccepts(top StreamID, hasTop bool, entry StreamID) bool {
	if entry == (StreamID{}) {
		return false
	}
	return !hasTop || top.Less(entry)
}
