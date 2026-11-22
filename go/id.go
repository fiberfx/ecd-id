// Package eid implements the Ecd ID, version 1: a 16-byte identifier of system (15 bits), kind (15), reserved
// (2), Unix milliseconds (48), writer (30) and count (18), whose canonical text is 28 lowercase characters such as
// emq_job_23obzcbapvupu2272222. The specification is in spec/ at the repository root; the vectors in vectors/ are
// the conformance contract.
//
// The package uses the standard library only. Clocks, lease stores, process ids and random sources are interfaces,
// so restarts, clocks set back and cloned processes can be exercised deterministically.
package eid

import "bytes"

// Alphabet holds the 32 symbols in value order; ASCII order equals value order.
const Alphabet = "234567abcdefghijklmnopqrstuvwxyz"

// Layout limits.
const (
	TextLen    = 28
	TimeMax    = 1<<48 - 1
	WriterMax  = 1<<30 - 1
	CountLimit = 1 << 18
)

const notInAlphabet = 0xff

var value = func() (t [256]byte) {
	for i := range t {
		t[i] = notInAlphabet
	}
	for i := 0; i < 32; i++ {
		t[Alphabet[i]] = byte(i)
	}
	return t
}()

// Code is a 15-bit code of three alphabet characters: a system code or a kind.
type Code uint16

// Class is the class of a system code, read from its first character.
type Class int

// The three classes.
const (
	Registered Class = iota // a letter: a row in the system registry
	Detached                // a digit 3-7: drawn at random by a producer with no lease
	Reserved                // the digit 2: reserved for a later format and refused
)

func (c Class) String() string {
	return [...]string{"registered", "detached", "reserved"}[c]
}

// ParseCode parses three alphabet characters. Uppercase and any other byte are refused.
func ParseCode(s string) (Code, bool) {
	if len(s) != 3 {
		return 0, false
	}
	var v uint16
	for i := 0; i < 3; i++ {
		x := value[s[i]]
		if x == notInAlphabet {
			return 0, false
		}
		v = v<<5 | uint16(x)
	}
	return Code(v), true
}

// MustCode parses a code known to be valid; it panics otherwise.
func MustCode(s string) Code {
	c, ok := ParseCode(s)
	if !ok {
		panic("eid: invalid code " + s)
	}
	return c
}

// Class returns the class of c read as a system code.
func (c Code) Class() Class {
	switch first := c >> 10; {
	case first == 0:
		return Reserved
	case first <= 5:
		return Detached
	default:
		return Registered
	}
}

// IsKind reports whether c is a valid kind: three letters.
func (c Code) IsKind() bool {
	return (c>>10)&31 >= 6 && (c>>5)&31 >= 6 && c&31 >= 6
}

func (c Code) String() string {
	return string([]byte{Alphabet[(c>>10)&31], Alphabet[(c>>5)&31], Alphabet[c&31]})
}

// Sys is the kind that names a system itself, in every system.
var Sys = MustCode("sys")

// ID is an Ecd ID: 16 bytes, big-endian. Comparing the bytes compares system, kind, time, writer and count.
type ID [16]byte

func (id ID) halves() (hi, lo uint64) {
	for i := 0; i < 8; i++ {
		hi = hi<<8 | uint64(id[i])
		lo = lo<<8 | uint64(id[8+i])
	}
	return hi, lo
}

func fromHalves(hi, lo uint64) (id ID) {
	for i := 7; i >= 0; i-- {
		id[i] = byte(hi)
		id[8+i] = byte(lo)
		hi >>= 8
		lo >>= 8
	}
	return id
}

func pack(system, kind Code, time uint64, writer, count uint32) ID {
	hi := uint64(system)<<49 | uint64(kind)<<34 | time>>16
	lo := (time&0xffff)<<48 | uint64(writer)<<18 | uint64(count)
	return fromHalves(hi, lo)
}

// FromParts builds an id from its fields, checking each against the layout.
func FromParts(system, kind Code, time uint64, writer, count uint32) (ID, error) {
	if system.Class() == Reserved {
		return ID{}, ErrSystemReserved
	}
	if !kind.IsKind() {
		return ID{}, ErrKind
	}
	if system >= 1<<15 || kind >= 1<<15 || time > TimeMax || writer > WriterMax || count >= CountLimit {
		return ID{}, ErrRange
	}
	return pack(system, kind, time, writer, count), nil
}

// SystemID returns the system's own id: its code, kind sys and a zero local part.
func SystemID(system Code) (ID, error) {
	return FromParts(system, Sys, 0, 0, 0)
}

func check(id ID) (ID, error) {
	hi, _ := id.halves()
	if (hi>>32)&3 != 0 {
		return ID{}, ErrReserved
	}
	if id.System().Class() == Reserved {
		return ID{}, ErrSystemReserved
	}
	if !id.Kind().IsKind() {
		return ID{}, ErrKind
	}
	return id, nil
}

// FromBytes is binary ingress: exactly 16 bytes, then steps 5-7 of the strict parse.
func FromBytes(b []byte) (ID, error) {
	if len(b) != 16 {
		return ID{}, ErrLength
	}
	var id ID
	copy(id[:], b)
	return check(id)
}

// Parse is the strict parse of the canonical text (spec/id.md#strict-parse).
func Parse(t []byte) (ID, error) {
	if len(t) != TextLen {
		return ID{}, ErrLength
	}
	if t[3] != '_' || t[7] != '_' {
		return ID{}, ErrSeparator
	}
	var hi, lo uint64
	var last byte
	i := 0
	for pos, c := range t {
		if pos == 3 || pos == 7 {
			continue
		}
		v := value[c]
		if v == notInAlphabet {
			return ID{}, ErrAlphabet
		}
		if i < 25 {
			hi = hi<<5 | lo>>59
			lo = lo<<5 | uint64(v)
		} else {
			last = v
		}
		i++
	}
	if last&3 != 0 {
		return ID{}, ErrPad
	}
	hi = hi<<3 | lo>>61
	lo = lo<<3 | uint64(last>>2)
	return check(fromHalves(hi, lo))
}

// ParseString parses the canonical text given as a string.
func ParseString(s string) (ID, error) {
	return Parse([]byte(s))
}

// low64 returns the low 64 bits of the 128-bit value shifted right by k.
func low64(hi, lo uint64, k uint) uint64 {
	switch {
	case k == 0:
		return lo
	case k >= 64:
		return hi >> (k - 64)
	default:
		return lo>>k | hi<<(64-k)
	}
}

// Encode returns the canonical text as 28 bytes.
func (id ID) Encode() [TextLen]byte {
	var out [TextLen]byte
	hi, lo := id.halves()
	out[3], out[7] = '_', '_'
	for i := 0; i < 26; i++ {
		var v uint64
		if i < 25 {
			v = low64(hi, lo, uint(123-5*i)) & 31
		} else {
			v = (lo & 7) << 2
		}
		pos := i
		if i >= 6 {
			pos = i + 2
		} else if i >= 3 {
			pos = i + 1
		}
		out[pos] = Alphabet[v]
	}
	return out
}

func (id ID) String() string {
	t := id.Encode()
	return string(t[:])
}

// Bytes returns the 16 bytes.
func (id ID) Bytes() []byte {
	b := id
	return b[:]
}

// System returns the system that minted the id.
func (id ID) System() Code {
	hi, _ := id.halves()
	return Code(hi >> 49)
}

// Kind returns the kind.
func (id ID) Kind() Code {
	hi, _ := id.halves()
	return Code((hi >> 34) & 0x7fff)
}

// Time returns the Unix milliseconds stamped by the minter.
func (id ID) Time() uint64 {
	hi, lo := id.halves()
	return (hi&0xffffffff)<<16 | lo>>48
}

// Writer returns the writer number.
func (id ID) Writer() uint32 {
	_, lo := id.halves()
	return uint32(lo>>18) & WriterMax
}

// Count returns the count inside the writer's millisecond.
func (id ID) Count() uint32 {
	_, lo := id.halves()
	return uint32(lo) & (CountLimit - 1)
}

// Class returns the class of the id's system.
func (id ID) Class() Class {
	return id.System().Class()
}

// Compare compares two ids in byte order.
func (id ID) Compare(o ID) int {
	return bytes.Compare(id[:], o[:])
}
