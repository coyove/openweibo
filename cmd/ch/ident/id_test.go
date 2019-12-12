package ident

import (
	"math/rand"
	"testing"
)

func TestID(t *testing.T) {
	for i := 0; i < 1e6; i++ {
		tag := SafeStringForCompressString(randomString())

		id := NewID(IDTagAuthor).SetTag(tag)
		if rand.Intn(2) == 0 {
			id = NewGeneralID()
		}

		id2 := ParseID(id.String())
		if id2 != id {
			t.Fatal("[", id, "][", id2, "]")
		}
	}
}
