package main

import "math/bits"

// The vectors that draw random fields (valid.tsv and stream.tsv) are reproducible from one seed. The generator is the
// 32-bit Mersenne Twister MT19937, seeded from an integer by init_by_array and read through getrandbits and randrange,
// the way CPython's random.Random does, so a given seed yields the same draws.

const (
	mtN     = 624
	mtM     = 397
	mtUpper = 0x80000000
	mtLower = 0x7fffffff
)

// mersenne is an MT19937 state.
type mersenne struct {
	state [mtN]uint32
	index int
}

func (r *mersenne) initGenrand(s uint32) {
	r.state[0] = s
	for i := 1; i < mtN; i++ {
		prev := r.state[i-1]
		r.state[i] = 1812433253*(prev^(prev>>30)) + uint32(i)
	}
	r.index = mtN
}

// newMersenne seeds a generator from a non-negative integer: init_by_array over the 32-bit words of the seed, least
// significant first.
func newMersenne(seed uint64) *mersenne {
	key := []uint32{uint32(seed)}
	if seed>>32 != 0 {
		key = append(key, uint32(seed>>32))
	}
	r := new(mersenne)
	r.initGenrand(19650218)
	i, j := 1, 0
	for k := max(mtN, len(key)); k > 0; k-- {
		prev := r.state[i-1]
		r.state[i] = (r.state[i] ^ ((prev ^ (prev >> 30)) * 1664525)) + key[j] + uint32(j)
		i++
		j++
		if i >= mtN {
			r.state[0] = r.state[mtN-1]
			i = 1
		}
		if j >= len(key) {
			j = 0
		}
	}
	for k := mtN - 1; k > 0; k-- {
		prev := r.state[i-1]
		r.state[i] = (r.state[i] ^ ((prev ^ (prev >> 30)) * 1566083941)) - uint32(i)
		i++
		if i >= mtN {
			r.state[0] = r.state[mtN-1]
			i = 1
		}
	}
	r.state[0] = mtUpper
	return r
}

// next returns the next 32-bit output.
func (r *mersenne) next() uint32 {
	if r.index >= mtN {
		mag := [2]uint32{0, 0x9908b0df}
		kk := 0
		for ; kk < mtN-mtM; kk++ {
			y := r.state[kk]&mtUpper | r.state[kk+1]&mtLower
			r.state[kk] = r.state[kk+mtM] ^ y>>1 ^ mag[y&1]
		}
		for ; kk < mtN-1; kk++ {
			y := r.state[kk]&mtUpper | r.state[kk+1]&mtLower
			r.state[kk] = r.state[kk+mtM-mtN] ^ y>>1 ^ mag[y&1]
		}
		y := r.state[mtN-1]&mtUpper | r.state[0]&mtLower
		r.state[mtN-1] = r.state[mtM-1] ^ y>>1 ^ mag[y&1]
		r.index = 0
	}
	y := r.state[r.index]
	r.index++
	y ^= y >> 11
	y ^= y << 7 & 0x9d2c5680
	y ^= y << 15 & 0xefc60000
	y ^= y >> 18
	return y
}

// getrandbits returns k random bits, 0 < k <= 64: the top k bits of one output when k <= 32, else two outputs, the first
// the low word and the second cut to the remaining bits.
func (r *mersenne) getrandbits(k uint) uint64 {
	assert(k > 0 && k <= 64, "getrandbits: 0 < k <= 64")
	if k <= 32 {
		return uint64(r.next() >> (32 - k))
	}
	low := uint64(r.next())
	high := uint64(r.next() >> (64 - k))
	return low | high<<32
}

// randbelow returns a random integer in [0, n) by rejection over getrandbits, for n > 0.
func (r *mersenne) randbelow(n uint64) uint64 {
	k := uint(bits.Len64(n))
	v := r.getrandbits(k)
	for v >= n {
		v = r.getrandbits(k)
	}
	return v
}

// randrange returns a random integer in [start, stop), for start < stop.
func (r *mersenne) randrange(start, stop uint64) uint64 {
	return start + r.randbelow(stop-start)
}
