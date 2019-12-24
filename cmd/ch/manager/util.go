package manager

import (
	"crypto/sha1"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	rxCrawler = regexp.MustCompile(`(?i)(bot|googlebot|crawler|spider|robot|crawling)`)
)

type KeyValueOp interface {
	Lock(string)
	Unlock(string)
	Get(string) ([]byte, error)
	Set(string, []byte) error
	Delete(string) error
}

func IsCrawler(g *gin.Context) bool {
	if rxCrawler.MatchString(g.Request.UserAgent()) {
		return true
	}
	v, _ := g.Cookie("crawler")
	return v == "1"
}

//func (m *Manager) IncrCounter(g *gin.Context, id string) {
//	if IsCrawler(g) {
//		return
//	}
//
//	m.dbCounter.Lock(id)
//	defer m.dbCounter.Unlock(id)
//
//	x, _ := m.counter.m.LoadOrStore(id, &[256]bool{})
//
//	ip := g.MustGet("ip").(net.IP)
//	(*x)[ip[len(ip)-1]] = true
//
//	count := 0
//	m.counter.m.Range(func(k, v interface{}) bool {
//		count++
//		if count > 64 {
//			go m.writeCounterToDB()
//			return false
//		}
//		return true
//	})
//
//	m.counter.k.Reschedule(func() { go m.writeCounterToDB() }, time.Second*10)
//}
//
//func (m *Manager) writeCounterToDB() {
//	m.counter.k.Cancel()
//	count := 0
//
//	m.counter.m.Range(func(k, v interface{}) bool {
//		count++
//		m.dbCounter.Get(
//		return true
//	})
//
//	m.counter.m = sync.Map{}
//	log.Println("[writeCounterToDB] sched:", count)
//}

func dec0(a *int32) {
	*a--
	if *a < 0 {
		*a = 0
	}
}

func MakeID(mth string, a, b string) string {
	switch mth {
	case "follow":
		return MakeFollowID(a, b)
	case "followed":
		return makeFollowedID(a, b)
	default:
		return makeBlockID(a, b)
	}
}

func MakeFollowID(from, to string) string {
	h := sha1.Sum([]byte(to))
	return "u/" + from + "/follow/" + strconv.Itoa(int(h[0]))
}

func makeFollowedID(from, to string) string {
	return "u/" + from + "/followed/" + to
}

func makeBlockID(from, to string) string {
	return "u/" + from + "/block/" + to
}

func makeLikeID(from, to string) string {
	return "u/" + from + "/like/" + to
}

func makeVoteID(from, aid string) string {
	return "u/" + from + "/vote/" + aid
}

func lastElemInCompID(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

func atoi64(a string) int64 {
	v, _ := strconv.ParseInt(a, 10, 64)
	return v
}

func atob(a string) bool {
	v, _ := strconv.ParseBool(a)
	return v
}
