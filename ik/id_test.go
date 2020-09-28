package ik

import (
	"crypto/aes"
	"math/rand"
	"testing"
	"time"

	"github.com/coyove/iis/common"
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

func TestOTT(t *testing.T) {
	common.Cfg.Blk, _ = aes.NewCipher(make([]byte, 16))
	tok := MakeOTT("a")
	t.Log(tok)
	t.Log(ValidateOTT("a", tok))
	t.Log(ValidateOTT("abb", tok))
}

func TestShortId(t *testing.T) {
	start := time.Now()
	common.Cfg.Blk, _ = aes.NewCipher(make([]byte, 16))

	for i := int64(0); i < 1e6; i++ {
		v := rand.Int63() & 0x7ffffffff
		// c := EncryptPhone(v)
		c, _ := FormatShortId(v)
		if x, err := ParseShortId(c); x != v {
			t.Fatal(c, "expect", v, "got", x, err)
		}
	}

	t.Log(time.Since(start))

	for i := 0; i < 1e1; i++ {
		c, _ := StringifyShortId(int64(i) + 1e8)
		t.Logf("%d %s", i, c)
	}
}
