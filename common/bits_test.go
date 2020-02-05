package common

import (
	"math/rand"
	"strconv"
	"testing"
	"time"
)

func TestBits(t *testing.T) {
	rand.Seed(time.Now().Unix())

	for x := 0; x < 1e4; x++ {
		n := map[string]string{}
		for i := 0; i < 256; i++ {
			if rand.Intn(2) == 1 {
				n[strconv.Itoa(i)] = "1"
			}
		}

		v := Pack256(n)
		m := Unpack256(v)
		for k, v := range m {
			if n[k] != v {
				t.Fatal(n, m)
			}
		}

		t.Log(v)
	}
}
