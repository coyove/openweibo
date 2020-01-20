package dal

import (
	"crypto/sha1"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/dal/kv/cache"
	"github.com/gin-gonic/gin"
)

var (
	rxCrawler = regexp.MustCompile(`(?i)(bot|googlebot|crawler|spider|robot|crawling)`)
)

type KeyValueOp interface {
	Get(string) ([]byte, error)
	Set(string, []byte) error
	SetGlobalCache(*cache.GlobalCache)
}

func IsCrawler(g *gin.Context) bool {
	if rxCrawler.MatchString(g.Request.UserAgent()) {
		return true
	}
	v, _ := g.Cookie("crawler")
	return v == "1"
}

//func  IncrCounter(g *gin.Context, id string) {
//	if IsCrawler(g) {
//		return
//	}
//
//	dbCounter.Lock(id)
//	defer dbCounter.Unlock(id)
//
//	x, _ := counter.LoadOrStore(id, &[256]bool{})
//
//	ip := g.MustGet("ip").(net.IP)
//	(*x)[ip[len(ip)-1]] = true
//
//	count := 0
//	counter.Range(func(k, v interface{}) bool {
//		count++
//		if count > 64 {
//			go writeCounterToDB()
//			return false
//		}
//		return true
//	})
//
//	counter.k.Reschedule(func() { go writeCounterToDB() }, time.Second*10)
//}
//
//func  writeCounterToDB() {
//	counter.k.Cancel()
//	count := 0
//
//	counter.Range(func(k, v interface{}) bool {
//		count++
//		dbCounter.Get(
//		return true
//	})
//
//	counter.m = sync.Map{}
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
		return makeFollowID(a, b)
	case "followed":
		return makeFollowedID(a, b)
	default:
		return makeBlockID(a, b)
	}
}

func makeFollowID(from, to string) string {
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

func makeCheckpointID(from string, t time.Time) string {
	return "u/" + from + "/checkpoint/" + t.Format("2006-01")
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
