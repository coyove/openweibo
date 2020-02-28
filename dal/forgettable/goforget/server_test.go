package goforget

import (
	"log"
	"testing"
	"time"

	"github.com/coyove/iis/dal/kv/cache"
)

func TestServer(t *testing.T) {
	Init(&cache.RedisConfig{
		Addr:         "devbox0:6379",
		BatchWorkers: 10,
	})
	time.Sleep(time.Second)
	Incr("root", "a", "b", "c")
	Incr("root", "a", "b", "d")
	time.Sleep(time.Second)
	Incr("root", "b", "c", "d")
	Incr("root", "b")

	res := TopN("root", 10)
	for k, r := range res.Data {
		log.Println(k, r.P)
	}
}
