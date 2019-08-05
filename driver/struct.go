package driver

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	ErrKeyNotFound  = errors.New("key not found")
	ErrFullCapacity = errors.New("storage: no space left")
	ErrThrottled    = errors.New("storage: resource temporarily throttled")
)

type TokenBucket struct {
	speed       int64 // bytes per second
	capacity    int64 // bytes
	maxCapacity int64
	timeout     time.Duration
	lastConsume time.Time
	mu          sync.Mutex
}

func NewTokenBucket(config string) *TokenBucket {
	var speed, max, wait int64
	if _, err := fmt.Sscanf(config, "%dx%d/%d", &speed, &wait, &max); err != nil {
		panic(err)
	}

	log.Println("[tokenbucket] speed:", speed, "b, max:", max, "b, timeout:", wait, "s")
	return &TokenBucket{
		speed:       speed,
		maxCapacity: max,
		timeout:     time.Duration(wait) * time.Second,
		lastConsume: time.Now(),
	}
}

func (tb *TokenBucket) String() string {
	return fmt.Sprintf("%dx%d/%d", tb.speed, tb.timeout/time.Second, tb.maxCapacity)
}

func (tb *TokenBucket) Consume(n int64) bool {
	if tb.speed == 0 {
		return true
	}

	tb.mu.Lock()
	now := time.Now()
	ms := now.Sub(tb.lastConsume).Nanoseconds() / 1e6
	tb.capacity += ms * tb.speed / 1000 // since 'ms' may be negative, the capacity may be decreased as well

	if tb.capacity > tb.maxCapacity {
		tb.capacity = tb.maxCapacity
	}

	if n <= tb.capacity {
		tb.lastConsume = now
		tb.capacity -= n
		tb.mu.Unlock()
		return true
	}

	sec := float64(n-tb.capacity) / float64(tb.speed)
	sleepTime := time.Duration(sec*1000) * time.Millisecond

	if sleepTime > tb.timeout {
		tb.mu.Unlock()
		return false
	}

	tb.capacity = 0
	tb.lastConsume = now.Add(sleepTime)
	tb.mu.Unlock()

	time.Sleep(sleepTime)
	return true
}
