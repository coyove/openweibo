package ident

import (
	"math/rand"
	"testing"
)

func TestID(t *testing.T) {
	for i := 0; i < 1e6; i++ {
		tag := SafeStringForCompressString(randomString())

		id := NewID(IDTagCategory).SetTag(tag)
		if rand.Intn(2) == 0 {
			id = id.SetReply(uint16(rand.Uint32()) | 1)
		}

		id2 := ParseID(id.String())
		if id2.Tag() != tag || id2.Header() != id.Header() || id.reply != id2.reply {
			t.Fatal("[", id, "][", id2, "]", id.Tag(), tag)
		}
	}
}
