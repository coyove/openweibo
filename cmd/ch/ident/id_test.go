package ident

import (
	"bytes"
	"crypto/aes"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
)

func TestID(t *testing.T) {
	rand.Seed(time.Now().Unix())
	ln := 0

	for i := 0; i < 1e6; i++ {
		id := NewID(HeaderAnnounce, strconv.Itoa(i))
		id.ctr = uint32(i)

		for {
			v := int16(rand.Uint64())&0x1ff + 1
			if !id.RIndexAppend(v) {
				break
			}
			if id.RIndex() != v {
				t.Fatal(id.rIndex, id.RIndex(), v)
			}
		}

		ln += id.RIndexLen(nil)

		id2 := ParseID(id.Marshal())

		if id2 != id {
			t.Fatal(id, id2)
		}
	}

	t.Log(ln/1e6, ln)
}

func init() {
	rand.Seed(time.Now().Unix())
	config.Cfg.Blk, _ = aes.NewCipher([]byte("0123456789abcdef"))
	config.Cfg.IDTokenTTL = 10
}

func BenchmarkTempToken(b *testing.B) {
	for i := 0; i < b.N; i++ {
		id := strconv.Itoa(rand.Int())
		xid := MakeTempToken(id)
		id2 := ParseTempToken(xid)
		if id != id2 {
			b.Fatal(id, "[", id2, "]")
		}
	}
}

func BenchmarkTempTokenAID(b *testing.B) {
	id := make([]byte, 30)
	key := [4]byte{byte(rand.Int()), byte(rand.Int()), byte(rand.Int()), byte(rand.Int())}
	for i := 0; i < b.N; i++ {
		xid := EncryptArticleID(id[:], key)
		id2 := DecryptArticleID(xid, key)
		if !bytes.Equal(id[:], id2) {
			b.Fatal(id, id2)
		}
	}
}
