package manager

import (
	"encoding/binary"
	"log"
	"net"
	"time"

	mv "github.com/coyove/iis/cmd/ch/modelview"
	"github.com/etcd-io/bbolt"
	"github.com/gin-gonic/gin"
)

func (m *Manager) IncrCounter(g *gin.Context, idbuf []byte) {
	id := string(idbuf)

	if m.counter.rx.MatchString(g.Request.UserAgent()) {
		return
	}

	ip := [4]byte{}
	copy(ip[:], g.MustGet("ip").(net.IP))
	ip32 := binary.BigEndian.Uint32(ip[:])

	m.mu.Lock()
	if m.counter.m[id] == nil {
		m.counter.m[id] = map[uint32]bool{ip32: true}
	} else {
		m.counter.m[id][ip32] = true
	}

	if len(m.counter.m) > 64 {
		go m.writeCounterToDB()
	} else {
		m.counter.k.Reschedule(func() { go m.writeCounterToDB() }, time.Second*30)
	}
	m.mu.Unlock()
}

func (m *Manager) writeCounterToDB() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counter.k.Cancel()
	n := len(m.counter.m)
	err := m.db.Update(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(bkPost)
		a := &mv.Article{}

		for id, hits := range m.counter.m {
			if err := a.UnmarshalA(bk.Get([]byte(id))); err == nil {
				a.Views += int64(len(hits))
				bk.Put([]byte(id), a.MarshalA())
			}
			delete(m.counter.m, id)
		}
		return nil
	})

	log.Println("[writeCounterToDB] sched:", n, err)
}
