package eid

// The brd14 layout (spec/registry.md#brd14): a namespace of three bytes A-Z, then 11 Base62 digits of a snowflake
// below 2^63 whose milliseconds count from Brd14Epoch.
const (
	Brd14Len   = 14
	Brd14Epoch = 1704067200000
)

const (
	brd14Alphabet   = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	brd14MaxPayload = "AzL8n0Y58m7"
)

var brdSystem = MustCode("brd")

var base62 = func() (t [256]byte) {
	for i := range t {
		t[i] = notInAlphabet
	}
	for i := 0; i < 62; i++ {
		t[brd14Alphabet[i]] = byte(i)
	}
	return t
}()

// FromBrd14 embeds a brd14 id into an id of the system brd: the kind is the namespace in lowercase, the time is
// Brd14Epoch plus the snowflake's milliseconds, the writer its node and the count its sequence. It reads no registry.
func FromBrd14(text string) (ID, error) {
	if len(text) != Brd14Len {
		return ID{}, ErrBrd14Length
	}
	var kind Code
	for i := 0; i < 3; i++ {
		c := text[i]
		if c < 'A' || c > 'Z' {
			return ID{}, ErrBrd14Namespace
		}
		kind = kind<<5 | Code(value[c-'A'+'a'])
	}
	for i := 3; i < Brd14Len; i++ {
		if base62[text[i]] == notInAlphabet {
			return ID{}, ErrBrd14Alphabet
		}
	}
	// Fixed width over an alphabet in ASCII order: byte order is numeric order.
	if text[3:] > brd14MaxPayload {
		return ID{}, ErrBrd14Range
	}
	var snow uint64
	for i := 3; i < Brd14Len; i++ {
		snow = snow*62 + uint64(base62[text[i]])
	}
	return pack(brdSystem, kind, Brd14Epoch+snow>>22, uint32(snow>>12)&1023, uint32(snow)&4095), nil
}

// ToBrd14 is the inverse of FromBrd14. It reads no registry. An id that binary ingress refuses is refused with the
// reason of that step.
func ToBrd14(id ID) (string, error) {
	if _, err := check(id); err != nil {
		return "", err
	}
	if id.System() != brdSystem {
		return "", ErrBrd14System
	}
	t := id.Time()
	if t < Brd14Epoch || t-Brd14Epoch >= 1<<41 {
		return "", ErrBrd14Time
	}
	if id.Writer() >= 1<<10 {
		return "", ErrBrd14Writer
	}
	if id.Count() >= 1<<12 {
		return "", ErrBrd14Count
	}
	snow := (t-Brd14Epoch)<<22 | uint64(id.Writer())<<12 | uint64(id.Count())
	var out [Brd14Len]byte
	kind := id.Kind().String()
	for i := 0; i < 3; i++ {
		out[i] = kind[i] - 'a' + 'A'
	}
	for i := Brd14Len - 1; i >= 3; i-- {
		out[i] = brd14Alphabet[snow%62]
		snow /= 62
	}
	return string(out[:]), nil
}

// FromBrd14 is the registry layer of FromBrd14: it also refuses a kind that has no row of the system brd, in any
// state, with ErrBrd14Kind.
func (r *Registry) FromBrd14(text string) (ID, error) {
	id, err := FromBrd14(text)
	if err != nil {
		return ID{}, err
	}
	if _, ok := r.Kind(brdSystem, id.Kind()); !ok {
		return ID{}, ErrBrd14Kind
	}
	return id, nil
}

// ToBrd14 is the registry layer of ToBrd14: it also refuses a kind that has no row of the system brd, in any state,
// with ErrBrd14Kind.
func (r *Registry) ToBrd14(id ID) (string, error) {
	text, err := ToBrd14(id)
	if err != nil {
		return "", err
	}
	if _, ok := r.Kind(brdSystem, id.Kind()); !ok {
		return "", ErrBrd14Kind
	}
	return text, nil
}
