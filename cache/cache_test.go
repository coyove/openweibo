package cache

import (
	"io"
	"io/ioutil"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coyove/common/sched"
)

func init() {
	rand.Seed(time.Now().Unix())
	WatchInterval = time.Second
	sched.Verbose = false
}

func TestCache(t *testing.T) {
	c := New("cache", 1024*1024, func(k string) (io.ReadCloser, error) {
		return ioutil.NopCloser(strings.NewReader(k)), nil
	})

	for i := 0; i < 1e5; i++ {
		c.Fetch(ioutil.Discard, strconv.Itoa(int(rand.Uint64())))
		//time.Sleep(time.Millisecond * 10)
	}

	select {}
}
