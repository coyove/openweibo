package tagrank

import (
	"testing"
	"time"

	"github.com/coyove/iis/dal/kv/cache"
)

func TestRank(t *testing.T) {
	Init(&cache.RedisConfig{
		Addr: "localhost:6379",
	})

	zzzT := time.Now()
	Update("zzz2", zzzT, 50)
	// Update("zzz", zzzT, 10)
	// Update("zzz", zzzT, 12)

	t.Log(TopN(10))
}
