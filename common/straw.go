package common

import (
	"math"
)

// Format: \d{2}\].+
// Range: 0 ~ 99
func StrawWeight(v string) float64 {
	if len(v) < 3 || v[2] != ']' {
		return 50
	}
	return float64(v[0]-'0')*10 + float64(v[1]-'0')
}

func Draw(v string, cands []string) (cand string) {
	if len(cands) == 0 {
		return ""
	}

	max := -math.MaxFloat64

	for _, n := range cands {
		s := math.Log(float64(Hash16(n))/65536) / StrawWeight(n)

		if s > max {
			max = s
			cand = n
		}
	}
	return
}

func RemoveFromStrings(s []string, v string) []string {
	for i := range s {
		if s[i] == v {
			s = append(s[:i], s[i+1:]...)
			return s
		}
	}
	return s
}

func Hash32(n string) (h uint32) {
	h = 2166136261
	for i := 0; i < len(n); i++ {
		h = h * 16777619
		h = h ^ uint32(n[i])
	}
	return
}

func Hash16(n string) (h uint16) {
	return uint16(Hash32(n))
}
