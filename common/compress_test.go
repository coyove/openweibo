package common

import (
	"bytes"
	"compress/gzip"
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

	t.Log(DecompressString(CompressString("阿是事ア1多万吨")))
	t.Log("" == DecompressString(CompressString("")))

	for i := 0; i < 1e7; i++ {
		ori := randomString()
		str := SafeStringForCompressString(ori)
		if x := DecompressString(CompressString(str)); x != str {
			t.Fatal("Ori:", ori, "Safe:", str, "Dec:", x)
		}
	}
}

func TestCompress2(t *testing.T) {
	rand.Seed(time.Now().Unix())

	p := bytes.Buffer{}
	n := 16000
	for i := 0; i < n; i++ {
		ori := SafeStringForCompressString(randomString())
		p.Write([]byte(ori))
		p.WriteByte(0)
		p.Write([]byte("    "))
	}

	comp := &bytes.Buffer{}
	w := gzip.NewWriter(comp)
	w.Write(p.Bytes())
	w.Close()

	t.Log(p.Len())
	t.Log(comp.Len())
}
