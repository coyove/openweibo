package mq

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
)

func TestMQ(t *testing.T) {
	os.Remove("1.db")
	db, err := New("1.db")
	if err != nil {
		panic(err)
	}

	count := 1000
	for i := 0; i < count; i++ {
		if err := db.PushBack([]byte(strconv.Itoa(i))); err != nil {
			panic(err)
		}
		if i%100 == 0 {
			log.Println("progress:", i)
		}
	}

	mcheck := map[string]bool{}
	m, _ := db.PopFront()
	for m, next, err := db.View(0, 7); err == nil; m, next, err = db.View(next, 10) {
		for _, m := range m {
			mcheck[string(m.Value)] = true
		}
	}
	m.PutBack()

	for i := 1; i < count; i++ {
		if !mcheck[strconv.Itoa(i)] {
			t.Fatal(mcheck)
		}
	}

	mcheck = map[string]bool{}
	mu := sync.Mutex{}
	stop := make(chan bool, runtime.NumCPU())

	read := func() {
		for {
			b, err := db.PopFront()
			if err != nil {
				if err != ErrEmptyQueue {
					t.Fatal(err)
				}
				break
			}

			mu.Lock()
			if mcheck[string(b.Value)] {
				t.Fatal("double consume")
			}
			mcheck[string(b.Value)] = true
			mu.Unlock()
		}
		stop <- true
	}

	for i := 0; i < runtime.NumCPU(); i++ {
		go read()
	}
	for i := 0; i < runtime.NumCPU(); i++ {
		<-stop
	}

	for i := 0; i < count; i++ {
		if !mcheck[strconv.Itoa(i)] {
			t.Fatal(mcheck)
		}
	}

	db.Close()
	os.RemoveAll("1.db")
}

func TestMQPutBack(t *testing.T) {
	os.Remove("1.db")
	db, err := New("1.db")
	if err != nil {
		panic(err)
	}

	prefetch = 10
	for i := 0; i < 100; i++ {
		if err := db.PushBack([]byte(strconv.Itoa(i))); err != nil {
			panic(err)
		}
	}

	db.PopFront()
	m, _ := db.PopFront()
	db.PopFront()
	m.PutBack()
	db.Close()

	db, err = New("1.db")
	if err != nil {
		panic(err)
	}
	m2, _ := db.PopFront()

	t.Log(m.key, m2.key) // should be 2 == 2

	db.Close()
	os.RemoveAll("1.db")
}
