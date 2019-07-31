package driver

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

var (
	ErrKeyNotFound  = errors.New("key not found")
	ErrFullCapacity = errors.New("storage: no space left")
	ErrThrottled    = errors.New("storage: resource temporarily throttled")
)

type Stat struct {
	TotalBytes     int64
	AvailableBytes int64
	DownloadBytes  int64
	UploadBytes    int64
	Ping           int64
	ObjectCount    int64
	Sealed         bool
	Error          error
}

type KV interface {
	Put(k string, v []byte) error
	Get(k string) ([]byte, error)
	Delete(k string) error
	Stat() Stat
}

func Itoi(a interface{}, defaultValue int64) int64 {
	if a == nil {
		return defaultValue
	}

	switch a := a.(type) {
	case float64:
		return int64(a)
	case int64:
		return a
	case int:
		return int64(a)
	}

	i, err := strconv.ParseInt(fmt.Sprint(a), 10, 64)
	if err != nil {
		return defaultValue
	}
	return i
}

func Itos(a interface{}, defaultValue string) string {
	if a == nil {
		return defaultValue
	}

	switch a := a.(type) {
	case string:
		return a
	}

	return fmt.Sprint(a)
}

type TokenBucket struct {
	Speed int64 // bytes per second

	capacity    int64 // bytes
	maxCapacity int64
	lastConsume time.Time
	mu          sync.Mutex
}

func NewTokenBucket(speed, max int64) *TokenBucket {
	return &TokenBucket{
		Speed:       speed,
		lastConsume: time.Now(),
		maxCapacity: max,
	}
}

func (tb *TokenBucket) Consume(n int64, timeout time.Duration) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	if tb.Speed == 0 {
		tb.lastConsume = now
		return true
	}

	ms := now.Sub(tb.lastConsume).Nanoseconds() / 1e6
	tb.capacity += ms * tb.Speed / 1000

	if tb.capacity > tb.maxCapacity {
		tb.capacity = tb.maxCapacity
	}

	if n <= tb.capacity {
		tb.lastConsume = now
		tb.capacity -= n
		return true
	}

	sec := float64(n-tb.capacity) / float64(tb.Speed)
	sleepTime := time.Duration(sec*1000) * time.Millisecond
	if sleepTime > timeout {
		return false
	}
	time.Sleep(sleepTime)

	tb.capacity = 0
	tb.lastConsume = time.Now()
	return true
}
