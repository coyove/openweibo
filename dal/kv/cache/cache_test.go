package cache

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"unsafe"
)

func BenchmarkCache(b *testing.B) {
	c := NewWeakCache(65536, time.Second)
	p := unsafe.Pointer(new(int))

	for i := 0; i < b.N; i++ {
		c.Add(strconv.Itoa(i), p)
	}
}

func TestCache(t *testing.T) {
	c := NewWeakCache(1<<20, time.Second*50)

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

func BenchmarkReaddirnames(b *testing.B) {
	// b.StopTimer()
	// os.MkdirAll("tmp", 0777)
	// for j := 0; j < 10240; j++ {
	// 	f, _ := os.Create("tmp/" + strconv.Itoa(j+10240))
	// 	f.Close()
	// }
	// b.StartTimer()

	for i := 0; i < b.N; i++ {
		dirs, _ := ioutil.ReadDir("tmp")
		for _, dir := range dirs {
			if !dir.IsDir() {
				continue
			}

			path := filepath.Join("tmp", dir.Name())
			for _, f := range func() (files []string) {
				if dir, _ := os.Open(path); dir != nil {
					files, _ = dir.Readdirnames(-1)
					dir.Close()
				}
				return
			}() {
				if f == "zzz" {
					b.Fatal(f)
				}
			}
		}
	}

}

func BenchmarkReadDir(b *testing.B) {
	// b.StopTimer()
	// os.MkdirAll("tmp", 0777)
	// for j := 0; j < 10240; j++ {
	// 	f, _ := os.Create("tmp/" + strconv.Itoa(j))
	// 	f.Close()
	// }
	// b.StartTimer()

	for i := 0; i < b.N; i++ {
		dirs, _ := ioutil.ReadDir("tmp")
		for _, dir := range dirs {
			if !dir.IsDir() {
				continue
			}

			path := filepath.Join("tmp", dir.Name())
			files, _ := ioutil.ReadDir(path)
			for _, f := range files {
				if f.Name() == "zzz" {
					b.Fatal(f)
				}
			}
		}
	}
}

func TestFGlobal(t *testing.T) {
	c := NewGlobalCache(100, &RedisConfig{Addr: "devbox0:6379"})
	t.Log(c.Get("u/zzz"))
}
