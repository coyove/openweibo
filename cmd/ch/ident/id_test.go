package ident

import (
	"crypto/aes"
	"math/rand"
	"testing"
	"time"
	"unsafe"

	"github.com/coyove/iis/cmd/ch/config"
)

func TestID(t *testing.T) {
	ln := 0

	for i := 0; i < 1e6; i++ {
		id := NewID()
		id.ctr = uint16(i)

		vs := []int16{}
		for {
			v := int16(rand.Uint64())&0x1ff + 1
			if !id.RIndexAppend(v) {
				break
			}
			if id.RIndex() != v {
				t.Fatal(id.rIndex, id.RIndex(), v)
			}
			vs = append(vs, v)
		}

		ln += id.RIndexLen(nil)

		id2 := ParseID(id.String())

		if id2 != id {
			t.Fatal(id, id2)
		}

		if id2.RIndex() != vs[len(vs)-1] {
			t.Fatal(id, id2)
		}
	}

	t.Log(ln/1e6, ln)
}

func TestRandomID(t *testing.T) {
	for i := 0; i < 1e6; i++ {
		id := NewID()
		rand.Read((*(*[IDLen]byte)(unsafe.Pointer(&id)))[:])

		ParseID(id.String())

		// Ensure they won't panic
		id.RIndexLen(nil)
		id.RIndexAppend(1)
		id.RIndex()
		id.RIndexParent()
	}
}

func init() {
	rand.Seed(time.Now().Unix())
	config.Cfg.Blk, _ = aes.NewCipher([]byte("0123456789abcdef"))
	config.Cfg.IDTokenTTL = 10
}
