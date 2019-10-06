package main

import (
	"math/rand"
	"strconv"
	"testing"
	"time"
)

func TestID(t *testing.T) {
	rand.Seed(time.Now().Unix())

	for i := 0; i < 1e6; i++ {
		id := ID{
			Hdr: IDHdrAnnounce,
			Tag: strconv.Itoa(i),
		}

		id.Fill()
		id.ctr = uint32(i)

		for i := 0; i < 4; i++ {
			id.RIndex[i] = int16(rand.Uint64()) & 0xfff
		}

		x := id.Marshal()

		var id2 ID
		id2.Unmarshal(x)

		if id2 != id {
			t.Fatal(id, id2)
		}
	}
}
