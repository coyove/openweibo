package storage

import (
	"log"
	"math/rand"
	"testing"
)

func TestBitmap(t *testing.T) {
	for i := 1; i < 20; i++ {
		b := make(Bitmap, 12)
		b.Set(0, i)
		t.Log(i, b)
		t.Log(b.FirstUnset(0))
	}
}

func TestVarint(t *testing.T) {
	for i := 0; i < 10; i++ {
		testFuzzyAlloc(t, rand.Intn(10)+10, rand.Intn(128)+1, rand.Intn(128)+1)
	}
}

func testFuzzyAlloc(t *testing.T, w, x, y int) {
	tr := NewBlock(w)
	m := map[string][]byte{}
	for {
		n := rand.Intn(x) + rand.Intn(y) + 1
		buf := tr.Alloc(n)
		if buf == nil {
			break
		}
		rand.Read(buf)
		m[string(buf)] = buf

		if rand.Intn(5) == 0 {
			i := 0
			for k, v := range m {
				if rand.Intn(len(m)-i) == 0 {
					tr.Free(v)
					delete(m, k)
					break
				}
				i++
			}
		}
	}

	for k, v := range m {
		if k != string(v) {
			t.Fatal([]byte(k), v)
		}
	}

	log.Println("finished")
}
