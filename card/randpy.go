package main

import "math"

// Same generator the engagement was tuned against: Python's Random,
// Mersenne Twister, seed as a 32-bit word.

type pyRand struct {
	mt    [624]uint32
	idx   int
	extra *float64
}

func newPyRand(seed uint32) *pyRand {
	r := &pyRand{}
	r.initBy([]uint32{seed})
	return r
}

func (r *pyRand) initGen(s uint32) {
	r.mt[0] = s
	for i := 1; i < 624; i++ {
		r.mt[i] = 1812433253*(r.mt[i-1]^(r.mt[i-1]>>30)) + uint32(i)
	}
}

// Python splits an int seed into 32-bit words and mixes them this way.
func (r *pyRand) initBy(key []uint32) {
	r.initGen(19650218)
	i, j := 1, 0
	k := 624
	if len(key) > k {
		k = len(key)
	}
	for ; k > 0; k-- {
		r.mt[i] = (r.mt[i] ^ ((r.mt[i-1] ^ (r.mt[i-1] >> 30)) * 1664525)) + key[j] + uint32(j)
		i++
		j++
		if i >= 624 {
			r.mt[0] = r.mt[623]
			i = 1
		}
		if j >= len(key) {
			j = 0
		}
	}
	for k = 623; k > 0; k-- {
		r.mt[i] = (r.mt[i] ^ ((r.mt[i-1] ^ (r.mt[i-1] >> 30)) * 1566083941)) - uint32(i)
		i++
		if i >= 624 {
			r.mt[0] = r.mt[623]
			i = 1
		}
	}
	r.mt[0] = 0x80000000
	r.idx = 624
}

func (r *pyRand) twist() {
	const n = 624
	const m = 397
	for i := 0; i < n; i++ {
		y := (r.mt[i] & 0x80000000) | (r.mt[(i+1)%n] & 0x7fffffff)
		r.mt[i] = r.mt[(i+m)%n] ^ (y >> 1)
		if y%2 != 0 {
			r.mt[i] ^= 0x9908b0df
		}
	}
}

func (r *pyRand) u32() uint32 {
	if r.idx >= 624 {
		r.twist()
		r.idx = 0
	}
	y := r.mt[r.idx]
	r.idx++
	y ^= y >> 11
	y ^= (y << 7) & 0x9d2c5680
	y ^= (y << 15) & 0xefc60000
	y ^= y >> 18
	return y
}

func (r *pyRand) random() float64 {
	a := r.u32() >> 5
	b := r.u32() >> 6
	return (float64(a)*67108864.0 + float64(b)) * (1.0 / 9007199254740992.0)
}

func pyMod(a, b float64) float64 {
	r := math.Mod(a, b)
	if r < 0 {
		r += b
	}
	return r
}

func (r *pyRand) uniform(a, b float64) float64 {
	return a + (b-a)*r.random()
}

func (r *pyRand) gauss(mu, sigma float64) float64 {
	if r.extra != nil {
		z := *r.extra
		r.extra = nil
		return mu + z*sigma
	}
	x2pi := r.random() * 2 * math.Pi
	g2rad := math.Sqrt(-2 * math.Log(1-r.random()))
	z := math.Cos(x2pi) * g2rad
	next := math.Sin(x2pi) * g2rad
	r.extra = &next
	return mu + z*sigma
}

// Python 3 round-half-to-even, used when the plot list is ordered.
func pyRound(v float64) int {
	if v < 0 {
		return -pyRound(-v)
	}
	i := math.Floor(v)
	frac := v - i
	n := int(i)
	if frac > 0.5 || (frac == 0.5 && n%2 != 0) {
		n++
	}
	return n
}
