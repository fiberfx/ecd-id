package ecdid

import "bytes"

// Brd is the system that holds brd14 embeddings.
var Brd = MustCode("brd")

// Brd14Epoch is brd14's epoch: 2024-01-01T00:00:00Z in Unix milliseconds.
const Brd14Epoch = 1_704_067_200_000

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var maxPayload = []byte("AzL8n0Y58m7")

func base62Value(c byte) (uint64, bool) {
	switch {
	case c >= '0' && c <= '9':
		return uint64(c - '0'), true
	case c >= 'A' && c <= 'Z':
		return uint64(c-'A') + 10, true
	case c >= 'a' && c <= 'z':
		return uint64(c-'a') + 36, true
	}
	return 0, false
}

// Brd14Decode decodes a brd14 id into its namespace and 63-bit number, refusing in brd14's order.
func Brd14Decode(t []byte) (ns [3]byte, snow uint64, err error) {
	if len(t) != 14 {
		return ns, 0, ErrBrd14Length
	}
	for i := 0; i < 3; i++ {
		if t[i] < 'A' || t[i] > 'Z' {
			return ns, 0, ErrBrd14Namespace
		}
	}
	payload := t[3:]
	for _, c := range payload {
		v, ok := base62Value(c)
		if !ok {
			return ns, 0, ErrBrd14Charset
		}
		snow = snow*62 + v
	}
	if bytes.Compare(payload, maxPayload) > 0 {
		return ns, 0, ErrBrd14Range
	}
	copy(ns[:], t[:3])
	return ns, snow, nil
}

// Brd14Encode encodes a namespace and a number below 2^63 as a brd14 id.
func Brd14Encode(ns [3]byte, snow uint64) string {
	var out [14]byte
	copy(out[:3], ns[:])
	for i := 13; i >= 3; i-- {
		out[i] = base62[snow%62]
		snow /= 62
	}
	return string(out[:])
}

// Embed returns the embedding of a brd14 id: system brd, the namespace in lowercase, and the number's fields.
func Embed(t []byte) (ID, error) {
	ns, snow, err := Brd14Decode(t)
	if err != nil {
		return ID{}, err
	}
	kind, ok := ParseCode(string(bytes.ToLower(ns[:])))
	if !ok {
		return ID{}, ErrBrd14Namespace
	}
	return pack(Brd, kind, snow>>22+Brd14Epoch, uint32(snow>>12)&1023, uint32(snow)&4095), nil
}

// Unembed returns the brd14 id that an embedding came from.
func Unembed(id ID) (string, error) {
	if id.System() != Brd || id.Time() < Brd14Epoch {
		return "", ErrNotEmbedding
	}
	t := id.Time() - Brd14Epoch
	if t >= 1<<41 || id.Writer() >= 1024 || id.Count() >= 4096 {
		return "", ErrNotEmbedding
	}
	var ns [3]byte
	copy(ns[:], bytes.ToUpper([]byte(id.Kind().String())))
	return Brd14Encode(ns, t<<22|uint64(id.Writer())<<12|uint64(id.Count())), nil
}
