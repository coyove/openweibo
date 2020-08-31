package model

import (
	"github.com/coyove/iis/dal/kv"
	"testing"
)

func TestSearch(t *testing.T) {
	Init(&kv.RedisConfig{
		Addr: "devbox0:6379",
	})
	Index("test", "1", "cyoyovte")
	Index("test", "3", "coyvote")
	Index("test", "2", "google-2")
	Index("test", "4", "中国")
	t.Log(Search("test", "中国", 0, 10))
}
