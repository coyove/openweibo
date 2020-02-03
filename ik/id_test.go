package ik

import (
	"math/rand"
	"testing"
)

func TestID(t *testing.T) {
	t.Log(NewID(IDFollowing, "澜沫"))
	t.Log(ParseID("S5tqMfCXJIL~").Time())
}

func TestCompressID(t *testing.T) {
	gen := func(n int) []ID {
		r := []ID{}
		for i := 0; i < n; i++ {
			id := NewGeneralID()
			id.ts += rand.Uint32() & 0xffffff
			r = append(r, id)
		}
		return r
	}

	for _, n := range []int{1, 4, 16, 64, 100, 1000, 10000} {
		buf := gen(n)
		payload := make([]byte, rand.Intn(4))
		rand.Read(payload)

		res := CombineIDs(payload, buf...)
		t.Log(len(buf)*9, len(res))

		buf2, payload2 := SplitIDs(res)
		for i := range buf {
			if buf[i] != buf2[i] {
				t.Fatal(buf[i], buf2[i])
			}
		}
		if string(payload) != string(payload2) {
			t.Fatal(payload, payload2)
		}
	}
}
