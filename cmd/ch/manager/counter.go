package manager

import (
	"github.com/gin-gonic/gin"
)

func IsCrawler(g *gin.Context) bool {
	if rxCrawler.MatchString(g.Request.UserAgent()) {
		return true
	}
	v, _ := g.Cookie("crawler")
	return v == "1"
}

func (m *Manager) IncrCounter(g *gin.Context, id string) {
	//	if IsCrawler(g) {
	//		return
	//	}
	//
	//	ip := [4]byte{}
	//	copy(ip[:], g.MustGet("ip").(net.IP))
	//	ip32 := binary.BigEndian.Uint32(ip[:])
	//
	//	m.mu.Lock()
	//	if m.counter.m[id] == nil {
	//		m.counter.m[id] = map[uint32]bool{ip32: true}
	//	} else {
	//		m.counter.m[id][ip32] = true
	//	}
	//
	//	if len(m.counter.m) > 64 {
	//		go m.writeCounterToDB()
	//	} else {
	//		m.counter.k.Reschedule(func() { go m.writeCounterToDB() }, time.Second*30)
	//	}
	//	m.mu.Unlock()
}

func (m *Manager) writeCounterToDB() {
	//	m.mu.Lock()
	//	defer m.mu.Unlock()
	//
	//	m.counter.k.Cancel()
	//	n := len(m.counter.m)
	//
	//	for id, hits := range m.counter.m {
	//		if err := a.UnmarshalA(bk.Get([]byte(id))); err == nil {
	//			a.Views += int64(len(hits))
	//			bk.Put([]byte(id), a.MarshalA())
	//		}
	//		delete(m.counter.m, id)
	//	}
	//
	//	log.Println("[writeCounterToDB] sched:", n, err)
}
