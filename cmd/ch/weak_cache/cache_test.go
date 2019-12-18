package weak_cache

import (
	"strconv"
	"testing"
	"time"
	"unsafe"
)

func BenchmarkCache(b *testing.B) {
	c := NewCache(65536, time.Second)
	p := unsafe.Pointer(new(int))

	for i := 0; i < b.N; i++ {
		c.Add(strconv.Itoa(i), p)
	}
}

func TestCache(t *testing.T) {
	c := NewCache(1<<20, time.Second*50)

	for i := 0; i < 1e7; i++ {
		p := new(int)
		*p = i
		c.Add(strconv.Itoa(i), unsafe.Pointer(p))
	}

	for i := 0; i < 1e7; i++ {
		p := c.Get(strconv.Itoa(i))
		if p != nil {
			if *(*int)(p) != i {
				t.Fatal(i, *(*int)(p))
			}
		}
	}

	t.Log(c.HitRatio())
}
