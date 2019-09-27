package main

import (
	"encoding/binary"
	"log"
	"net"
	"regexp"
	"sync"
	"time"

	"github.com/coyove/common/sched"
	"github.com/etcd-io/bbolt"
	"github.com/gin-gonic/gin"
)

var counter = struct {
	mu sync.Mutex
	m  map[int64]map[uint32]bool
	k  sched.SchedKey
	rx *regexp.Regexp
}{
	m:  map[int64]map[uint32]bool{},
	rx: regexp.MustCompile(`(?i)(bot|googlebot|crawler|spider|robot|crawling)`),
}

func incrCounter(g *gin.Context, id int64) {
	if counter.rx.MatchString(g.Request.UserAgent()) {
		return
	}

	ip := [4]byte{}
	copy(ip[:], g.MustGet("ip").(net.IP))
	ip32 := binary.BigEndian.Uint32(ip[:])

	counter.mu.Lock()
	if counter.m[id] == nil {
		counter.m[id] = map[uint32]bool{ip32: true}
	} else {
		counter.m[id][ip32] = true
	}

	if len(counter.m) > 64 {
		go writeCounterToDB()
	} else {
		counter.k.Reschedule(func() { go writeCounterToDB() }, time.Second*30)
	}
	counter.mu.Unlock()
}

func writeCounterToDB() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	counter.k.Cancel()
	n := len(counter.m)
	err := m.db.Update(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(bkPost)
		a := &Article{}

		for id, hits := range counter.m {
			if err := a.unmarshal(bk.Get(idBytes(id))); err == nil {
				a.Views += int64(len(hits))
				bk.Put(idBytes(id), a.marshal())
			}
			delete(counter.m, id)
		}
		return nil
	})

	log.Println("[writeCounterToDB] sched:", n, err)
}
