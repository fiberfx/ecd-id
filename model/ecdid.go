// Command model is the reference model of the Ecd ID, version 1, and the generator of the vectors in vectors/ (Go
// standard library only).
//
// The model is the oracle: vectors.go uses it to generate every file in vectors/, and the Rust and Go libraries must
// agree with those files byte for byte. It imports nothing from the library in ../go, and it favours plain
// arithmetic over speed.
package main

import (
	"fmt"
	"math/big"
	"strings"
)

const (
	alphabet        = "234567abcdefghijklmnopqrstuvwxyz"
	textLen         = 28
	timeMax         = 1<<48 - 1
	writerMax       = 1<<30 - 1
	countLimit      = 1 << 18
	leaseTermMax    = 86_400_000
	secondSlots     = 1 << 17
	indexSlots      = 1 << 13
	brd14Epoch      = 1_704_067_200_000
	brd14Alphabet   = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	brd14MaxPayload = "AzL8n0Y58m7"
)

// Refused is a refusal with its named reason.
type Refused struct{ Reason string }

func (r *Refused) Error() string { return r.Reason }

func refuse(reason string) error { return &Refused{reason} }

// reason returns the reason of a refusal; any other error is a fault of the model and panics.
func reason(err error) string {
	r, ok := err.(*Refused)
	if !ok {
		panic(err)
	}
	return r.Reason
}

func assert(ok bool, what string) {
	if !ok {
		panic("assertion failed: " + what)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// value returns the value of an alphabet character.
func value(c byte) (uint64, bool) {
	i := strings.IndexByte(alphabet, c)
	return uint64(i), i >= 0
}

// extract returns the bits of n selected by mask after a right shift.
func extract(n *big.Int, shift uint, mask uint64) uint64 {
	v := new(big.Int).Rsh(n, shift)
	return v.And(v, new(big.Int).SetUint64(mask)).Uint64()
}

// code turns three alphabet characters into a 15-bit number.
func code(s string) (uint64, error) {
	if len(s) != 3 {
		return 0, refuse("code")
	}
	var v uint64
	for i := 0; i < 3; i++ {
		x, ok := value(s[i])
		if !ok {
			return 0, refuse("code")
		}
		v = v<<5 | x
	}
	return v, nil
}

func mustCode(s string) uint64 { return must(code(s)) }

func codeText(v uint64) string {
	return string([]byte{alphabet[(v>>10)&31], alphabet[(v>>5)&31], alphabet[v&31]})
}

var (
	brdSystem = mustCode("brd") // the system that holds brd14 embeddings
	sysKind   = mustCode("sys") // the kind of a system's own id
)

func systemClass(system uint64) string {
	first := system >> 10
	switch {
	case first == 0:
		return "reserved"
	case first <= 5:
		return "detached"
	}
	return "registered"
}

func kindValid(kind uint64) bool {
	for _, s := range []uint{10, 5, 0} {
		if (kind>>s)&31 < 6 {
			return false
		}
	}
	return true
}

// pack builds the 16 bytes of an id; the reserved bits are zero.
func pack(system, kind, time, writer, count uint64) []byte {
	assert(system < 1<<15 && kind < 1<<15, "pack: system and kind are 15 bits")
	assert(time <= timeMax && writer <= writerMax && count < countLimit, "pack: time, writer and count in range")
	n := new(big.Int)
	for _, p := range []struct {
		v     uint64
		shift uint
	}{{system, 113}, {kind, 98}, {time, 48}, {writer, 18}, {count, 0}} {
		n.Or(n, new(big.Int).Lsh(new(big.Int).SetUint64(p.v), p.shift))
	}
	return n.FillBytes(make([]byte, 16))
}

// fieldSet holds the fields of an id.
type fieldSet struct{ system, kind, reserved, time, writer, count uint64 }

func fields(b []byte) fieldSet {
	n := new(big.Int).SetBytes(b)
	return fieldSet{
		system:   extract(n, 113, 1<<15-1),
		kind:     extract(n, 98, 1<<15-1),
		reserved: extract(n, 96, 3),
		time:     extract(n, 48, timeMax),
		writer:   extract(n, 18, writerMax),
		count:    extract(n, 0, countLimit-1),
	}
}

func encode(b []byte) string {
	n := new(big.Int).Lsh(new(big.Int).SetBytes(b), 2)
	s := make([]byte, 26)
	for i := range s {
		s[i] = alphabet[extract(n, uint(5*(25-i)), 31)]
	}
	return string(s[:3]) + "_" + string(s[3:6]) + "_" + string(s[6:])
}

// checkBinary is binary ingress: the steps of the strict parse that apply to 16 bytes.
func checkBinary(b []byte) ([]byte, error) {
	if len(b) != 16 {
		return nil, refuse("length")
	}
	f := fields(b)
	if f.reserved != 0 {
		return nil, refuse("reserved")
	}
	if systemClass(f.system) == "reserved" {
		return nil, refuse("system-reserved")
	}
	if !kindValid(f.kind) {
		return nil, refuse("kind")
	}
	return append([]byte(nil), b...), nil
}

// parse is the strict parse of the canonical text, given as bytes. It returns 16 bytes or a refusal.
func parse(t []byte) ([]byte, error) {
	if len(t) != textLen {
		return nil, refuse("length")
	}
	if t[3] != '_' || t[7] != '_' {
		return nil, refuse("separator")
	}
	s := append(append(append([]byte(nil), t[:3]...), t[4:7]...), t[8:]...)
	n := new(big.Int)
	for _, c := range s {
		v, ok := value(c)
		if !ok {
			return nil, refuse("alphabet")
		}
		n.Lsh(n, 5)
		n.Or(n, new(big.Int).SetUint64(v))
	}
	if extract(n, 0, 3) != 0 {
		return nil, refuse("pad")
	}
	b := n.Rsh(n, 2).FillBytes(make([]byte, 16))
	if _, err := checkBinary(b); err != nil {
		return nil, err
	}
	assert(encode(b) == string(t), "parse: the canonical text round-trips")
	return b, nil
}

// ---- the Valkey stream-id mapping

func streamID(b []byte) (ms, seq uint64) {
	f := fields(b)
	return f.time, f.system<<48 | f.writer<<18 | f.count
}

func streamText(b []byte) string {
	ms, seq := streamID(b)
	return fmt.Sprintf("%d-%d", ms, seq)
}

// legacyTail is the 48-bit tail without the system (bytes 10-15), kept only as the control for collisions.
func legacyTail(b []byte) uint64 {
	f := fields(b)
	return f.writer<<18 | f.count
}

// decimal parses a canonical decimal: ASCII digits, no leading zero, below 2^64.
func decimal(p string) (uint64, bool) {
	if p == "" || (len(p) > 1 && p[0] == '0') {
		return 0, false
	}
	var v uint64
	for i := 0; i < len(p); i++ {
		if p[i] < '0' || p[i] > '9' {
			return 0, false
		}
		d := uint64(p[i] - '0')
		if v > (1<<64-1-d)/10 {
			return 0, false
		}
		v = v*10 + d
	}
	return v, true
}

func parseStreamText(s string) (ms, seq uint64, err error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, refuse("stream-syntax")
	}
	ms, okMS := decimal(parts[0])
	seq, okSeq := decimal(parts[1])
	if !okMS || !okSeq {
		return 0, 0, refuse("stream-syntax")
	}
	return ms, seq, nil
}

// fromStream is the inverse of streamID, given the kind that the stream carries.
func fromStream(kindText string, ms, seq uint64) ([]byte, error) {
	kind, err := code(kindText)
	if err != nil || !kindValid(kind) {
		return nil, refuse("kind")
	}
	if ms > timeMax || seq >= 1<<63 {
		return nil, refuse("stream-range")
	}
	system := seq >> 48
	if systemClass(system) == "reserved" {
		return nil, refuse("system-reserved")
	}
	return pack(system, kind, ms, (seq>>18)&writerMax, seq&(countLimit-1)), nil
}

// valkeyAccepts is Valkey's rule for XADD with an explicit id: the id must be greater than the stream's top, and not
// 0-0. A nil top is an empty stream.
func valkeyAccepts(top *[2]uint64, entry [2]uint64) bool {
	if entry == [2]uint64{0, 0} {
		return false
	}
	return top == nil || entry[0] > top[0] || (entry[0] == top[0] && entry[1] > top[1])
}

// ---- the system's own id (the mxdb owner)

func systemID(systemText string) ([]byte, error) {
	system, err := code(systemText)
	if err != nil {
		return nil, err
	}
	if systemClass(system) == "reserved" {
		return nil, refuse("system-reserved")
	}
	return pack(system, sysKind, 0, 0, 0), nil
}

// ---- leases

func writerFor(second, index uint64) uint64 {
	assert(index < indexSlots, "writerFor: index in range")
	return (second%secondSlots)<<13 | index
}

// lease is a grant: one writer number for a window of id-time, in milliseconds, of one system.
type lease struct{ system, writer, notBefore, notAfter uint64 }

func parseLease(s string) (lease, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 4 {
		return lease{}, refuse("lease-syntax")
	}
	system, err := code(parts[0])
	if err != nil {
		return lease{}, refuse("lease-syntax")
	}
	var n [3]uint64
	for i, p := range parts[1:] {
		v, ok := decimal(p)
		if !ok {
			return lease{}, refuse("lease-syntax")
		}
		n[i] = v
	}
	writer, nb, na := n[0], n[1], n[2]
	if systemClass(system) == "reserved" {
		return lease{}, refuse("lease-system")
	}
	if writer > writerMax {
		return lease{}, refuse("lease-writer")
	}
	if nb == 0 || na < nb || na > timeMax || na-nb > leaseTermMax {
		return lease{}, refuse("lease-window")
	}
	return lease{system, writer, nb, na}, nil
}

func leaseText(l lease) string {
	return fmt.Sprintf("%s:%d:%d:%d", codeText(l.system), l.writer, l.notBefore, l.notAfter)
}

// minter is the minter of the section "The minter" in spec/lease.md.
type minter struct {
	system, writer, notBefore, notAfter uint64
	lastT                               uint64
	hasLast                             bool
	count                               uint64
}

func newMinter(l lease) *minter {
	return &minter{system: l.system, writer: l.writer, notBefore: l.notBefore, notAfter: l.notAfter}
}

func (m *minter) mint(clockMS uint64, kindText string) ([]byte, error) {
	kind, err := code(kindText)
	if err != nil || !kindValid(kind) {
		return nil, refuse("kind")
	}
	t := max(clockMS, m.notBefore)
	if m.hasLast {
		t = max(t, m.lastT)
	}
	var count uint64
	if m.hasLast && t == m.lastT {
		count = m.count + 1
	}
	if count == countLimit {
		t, count = t+1, 0
	}
	if t > m.notAfter {
		return nil, refuse("lease-ended")
	}
	m.lastT, m.hasLast, m.count = t, true, count
	return pack(m.system, kind, t, m.writer, count), nil
}

func (m *minter) switchTo(l lease) error {
	if l.system != m.system {
		return refuse("lease-system")
	}
	nb := l.notBefore
	if m.hasLast {
		nb = max(nb, m.lastT+1)
	}
	if nb > l.notAfter {
		return refuse("lease-ended")
	}
	m.writer, m.notBefore, m.notAfter = l.writer, nb, l.notAfter
	return nil
}

// ---- brd14 embedding under the system brd

func brd14Decode(t []byte) (ns string, snow uint64, err error) {
	if len(t) != 14 {
		return "", 0, refuse("brd14-length")
	}
	for _, c := range t[:3] {
		if c < 0x41 || c > 0x5A {
			return "", 0, refuse("brd14-namespace")
		}
	}
	payload := string(t[3:])
	for i := 0; i < len(payload); i++ {
		if strings.IndexByte(brd14Alphabet, payload[i]) < 0 {
			return "", 0, refuse("brd14-charset")
		}
	}
	if payload > brd14MaxPayload {
		return "", 0, refuse("brd14-range")
	}
	for i := 0; i < len(payload); i++ {
		snow = snow*62 + uint64(strings.IndexByte(brd14Alphabet, payload[i]))
	}
	return string(t[:3]), snow, nil
}

func brd14Encode(ns string, snow uint64) string {
	digits := make([]byte, 11)
	for i := len(digits) - 1; i >= 0; i-- {
		digits[i] = brd14Alphabet[snow%62]
		snow /= 62
	}
	return ns + string(digits)
}

func embed(t []byte) ([]byte, error) {
	ns, snow, err := brd14Decode(t)
	if err != nil {
		return nil, err
	}
	return pack(brdSystem, mustCode(strings.ToLower(ns)), (snow>>22)+brd14Epoch, (snow>>12)&1023, snow&4095), nil
}

func unembed(b []byte) (string, error) {
	f := fields(b)
	t := int64(f.time) - brd14Epoch
	if f.system != brdSystem || t < 0 || t >= 1<<41 || f.writer >= 1024 || f.count >= 4096 {
		return "", refuse("not-embedding")
	}
	return brd14Encode(strings.ToUpper(codeText(f.kind)), uint64(t)<<22|f.writer<<12|f.count), nil
}
