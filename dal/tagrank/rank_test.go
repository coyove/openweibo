package tagrank

import (
	"github.com/coyove/iis/dal/storage"
	"testing"
	"time"
)

func TestRank(t *testing.T) {
	Init(&storage.RedisConfig{
		Addr: "localhost:6379",
	})

	zzzT := time.Now()
	Update("zzz2", zzzT, 50)
	// Update("zzz", zzzT, 10)
	// Update("zzz", zzzT, 12)

	t.Log(TopN(10))
}
