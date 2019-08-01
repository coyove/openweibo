package cache

import (
	"math/rand"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().Unix())
	WatchInterval = time.Second
}

func TestCache(t *testing.T) {
	c := New("cache", 10)
	_ = c.factor
}
