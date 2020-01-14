package common

import (
	"math/rand"
	"strconv"
	"testing"
)

func TestDraw(t *testing.T) {
	t.Log(Draw("aa", []string{"99]1", "29xas", "99]3"}))

	m := [256]int{}
	for i := 0; i < 1e6; i++ {
		v := strconv.Itoa(rand.Int())
		h := Hash32(v)
		m[h&0xff]++
	}

	t.Log(m)
}

func BenchmarkDraw(b *testing.B) {
	x := []string{"99]1", "29xas", "99]3"}
	for i := 0; i < b.N; i++ {
		Draw("aa", x)
	}
}
