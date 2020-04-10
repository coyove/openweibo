package dal

import (
	"crypto/sha1"
	"reflect"
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
	Get2(string, string) ([]byte, error)
	Set(string, []byte) error
	Set2(string, string, []byte) error
	SetGlobalCache(*cache.GlobalCache)
	Range(string, string, int) ([][]byte, string, error)
}

func IsCrawler(g *gin.Context) bool {
	if rxCrawler.MatchString(g.Request.UserAgent()) {
		return true
	}
	v, _ := g.Cookie("crawler")
	return v == "1"
}

func incdec(a *int32, b *int, inc bool) {
	if a != nil && inc {
		*a++
	} else if a != nil && !inc {
		if *a--; *a < 0 {
			*a = 0
		}
	} else if b != nil && inc {
		*b++
	} else if b != nil && !inc {
		if *b--; *b < 0 {
			*b = 0
		}
	}
}

func makeFollowID(from, to string) string {
	h := sha1.Sum([]byte(to))
	return "u/" + from + "/follow/" + strconv.Itoa(int(h[0]))
}

func makeFollowerAcceptanceID(from, to string) string {
	return "u/" + from + "/accept-follow/" + to
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

func setIfValid(k interface{}, v interface{}) {
	rk, rv := reflect.ValueOf(k), reflect.ValueOf(v)
	if rv.Pointer() == 0 {
		return
	}
	rk.Elem().Set(rv.Elem())
}
