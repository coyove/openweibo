package types

// Package hyperloglog implements the HyperLogLog and HyperLogLog++ cardinality
// estimation algorithms.
// These algorithms are used for accurately estimating the cardinality of a
// multiset using constant memory. HyperLogLog++ has multiple improvements over
// HyperLogLog, with a much lower error rate for smaller cardinalities.
//
// HyperLogLog is described here:
// http://algo.inria.fr/flajolet/Publications/FlFuGaMe07.pdf
//
// HyperLogLog++ is described here:
// http://research.google.com/pubs/pub40671.html

import (
	"math"
)

const (
	hlltwo32     = 1 << 32
	hllPrecision = 12
	HLLSize      = 1 << hllPrecision
)

type HyperLogLog []byte

// New returns a new initialized HyperLogLog.
func NewHyperLogLog() HyperLogLog {
	return HyperLogLog(make([]uint8, HLLSize))
}

// Add adds a new item to HyperLogLog h.
func (h HyperLogLog) Add(x uint32) {
	i := eb32(x, 32, 32-hllPrecision)          // {x31,...,x32-p}
	w := x<<hllPrecision | 1<<(hllPrecision-1) // {x32-p,...,x0}

	zeroBits := clz32(w) + 1
	if zeroBits > h[i] {
		h[i] = zeroBits
	}
}

// Merge takes another HyperLogLog and combines it with HyperLogLog h.
func (h HyperLogLog) Merge(other HyperLogLog) error {
	for i, v := range other {
		if v > h[i] {
			h[i] = v
		}
	}
	return nil
}

// Count returns the cardinality estimate.
func (h HyperLogLog) Count() uint64 {
	est := calculateEstimate(h)
	if est <= float64(HLLSize)*2.5 {
		if v := countZeros(h); v != 0 {
			return uint64(linearCounting(HLLSize, v))
		}
		return uint64(est)
	} else if est < hlltwo32/30 {
		return uint64(est)
	}
	return uint64(-hlltwo32 * math.Log(1-est/hlltwo32))
}

func alpha(m uint32) float64 {
	if m == 16 {
		return 0.673
	} else if m == 32 {
		return 0.697
	} else if m == 64 {
		return 0.709
	}
	return 0.7213 / (1 + 1.079/float64(m))
}

var clzLookup = []uint8{
	32, 31, 30, 30, 29, 29, 29, 29, 28, 28, 28, 28, 28, 28, 28, 28,
}

// This optimized clz32 algorithm is from:
// 	 http://embeddedgurus.com/state-space/2014/09/
// 	 		fast-deterministic-and-portable-counting-leading-zeros/
func clz32(x uint32) uint8 {
	var n uint8

	if x >= (1 << 16) {
		if x >= (1 << 24) {
			if x >= (1 << 28) {
				n = 28
			} else {
				n = 24
			}
		} else {
			if x >= (1 << 20) {
				n = 20
			} else {
				n = 16
			}
		}
	} else {
		if x >= (1 << 8) {
			if x >= (1 << 12) {
				n = 12
			} else {
				n = 8
			}
		} else {
			if x >= (1 << 4) {
				n = 4
			} else {
				n = 0
			}
		}
	}
	return clzLookup[x>>n] - n
}

// Extract bits from uint32 using LSB 0 numbering, including lo.
func eb32(bits uint32, hi uint8, lo uint8) uint32 {
	m := uint32(((1 << (hi - lo)) - 1) << lo)
	return (bits & m) >> lo
}

func linearCounting(m uint32, v uint32) float64 {
	fm := float64(m)
	return fm * math.Log(fm/float64(v))
}

func countZeros(s []uint8) uint32 {
	var c uint32
	for _, v := range s {
		if v == 0 {
			c++
		}
	}
	return c
}

func calculateEstimate(s []uint8) float64 {
	sum := 0.0
	for _, val := range s {
		sum += 1.0 / float64(uint64(1)<<val)
	}

	m := uint32(len(s))
	fm := float64(m)
	return alpha(m) * fm * fm / sum
}
