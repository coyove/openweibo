package id

import (
	"math/rand"
	"strconv"
	"testing"
	"time"
)

func TestID(t *testing.T) {
	rand.Seed(time.Now().Unix())

	for i := 0; i < 1e6; i++ {
		id := NewID(HeaderAnnounce, strconv.Itoa(i))
		id.ctr = uint32(i)

		for i := 0; i < 4; i++ {
			id.RIndexAppend(int16(rand.Uint64()) & 0xfff)
		}

		x := id.String()

		id2 := ParseID(x)

		if id2 != id {
			t.Fatal(id, id2)
		}
	}
}
