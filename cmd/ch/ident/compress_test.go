package ident

import (
	"math/rand"
	"testing"
	"time"
)

func randomString() string {
	ln := rand.Intn(4) + 8
	buf := make([]rune, ln)
	for i := range buf {
		switch rand.Intn(4) {
		case 0:
			buf[i] = rune(rand.Uint64() % 0x7f)
		case 1, 2:
			buf[i] = rune(rand.Uint64() % 0x9fff)
		default:
			buf[i] = rune(rand.Uint64() % 0x10ffff)
		}
	}
	return string(buf)
}

func TestCompress(t *testing.T) {
	rand.Seed(time.Now().Unix())

	t.Log(DecompressString12(CompressString12("阿是事ア1多万吨")))
	t.Log("" == DecompressString12(CompressString12("")))

	for i := 0; i < 1e7; i++ {
		ori := randomString()
		str := SafeStringForCompressString12(ori)
		if x := DecompressString12(CompressString12(str)); x != str {
			t.Fatal("Ori:", ori, "Safe:", str, "Dec:", x)
		}
	}
}
