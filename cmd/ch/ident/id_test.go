package ident

import (
	"math/rand"
	"strconv"
	"testing"
	"time"
)

func TestID(t *testing.T) {
	rand.Seed(time.Now().Unix())
	ln := 0

	for i := 0; i < 1e6; i++ {
		id := NewID(HeaderAnnounce, strconv.Itoa(i))
		id.ctr = uint32(i)

		for {
			v := int16(rand.Uint64())&0xff + 1
			if !id.RIndexAppend(v) {
				break
			}
			if id.RIndex() != v {
				t.Fatal(id.rIndex, id.RIndex(), v)
			}
		}

		x := id.String()
		ln += id.RIndexLen()

		id2 := ParseID(x)

		if id2 != id {
			t.Fatal(id, id2)
		}
	}

	t.Log(ln/1e6, ln)
}
