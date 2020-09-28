package ctr

import (
	"log"
	"os"
	"runtime"
	"sync"
	"testing"
)

func TestCtr(t *testing.T) {
	os.Mkdir("test", 0777)
	runtime.GOMAXPROCS(runtime.NumCPU())

	res := map[int64]bool{}
	var resmu sync.Mutex

	m := &MemBack{m: map[int64]int64{}}
	f := &FSBack{dir: "test"}
	_, _ = m, f

	clients := []*Counter{}
	for i := 0; i < 32; i++ {
		c := New(78, f) //  &fs{dir: "test"})
		clients = append(clients, c)
		go func() {
			for i := 0; i < 1e2; i++ {
				v, _ := c.Get()
				resmu.Lock()
				if res[v] {
					log.Println(m.m)
					panic(1)
				}
				res[v] = true
				resmu.Unlock()

				log.Println(v)
			}
		}()
	}

	select {}
	// fmt.Println(len(res), survey.totalTries, survey.totalDecls)
}
