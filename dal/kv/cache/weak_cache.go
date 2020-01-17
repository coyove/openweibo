package cache

import (
	"reflect"
	"sync/atomic"
	"time"
	"unsafe"
)

type slot struct {
	key uint64
	p   unsafe.Pointer
	die int64
}

type WeakCache struct {
	ttl          time.Duration
	m            []slot
	hits, misses int64
}

func NewWeakCache(size int, ttl time.Duration) *WeakCache {
	l := &WeakCache{
		ttl: ttl,
		m:   make([]slot, size),
	}
	return l
}

func (c *WeakCache) Add(key string, p unsafe.Pointer) {
	if p == nil {
		return
	}

	i := c.hashString(key)
	c.m[i%uint64(len(c.m))] = slot{
		key: i,
		p:   p,
		die: time.Now().Add(c.ttl).UnixNano(),
	}
}

func (c *WeakCache) Get(key string) unsafe.Pointer {
	i := c.hashString(key)
	if s := c.m[i%uint64(len(c.m))]; s.key == i {
		if time.Now().UnixNano() < s.die {
			atomic.AddInt64(&c.hits, 1)
			return s.p
		}
	}

	atomic.AddInt64(&c.misses, 1)
	return nil
}

func (c *WeakCache) Delete(key string) {
	i := c.hashString(key)
	if s := &c.m[i%uint64(len(c.m))]; s.key == i {
		*s = slot{}
	}
}

func (c *WeakCache) HitRatio() float64 {
	h, m := atomic.LoadInt64(&c.hits), atomic.LoadInt64(&c.misses)
	return float64(h) / (float64(h) + float64(m) + 1)
}

func (c *WeakCache) hashString(s string) uint64 {
	hash := uint64(14695981039346656037)
	hdr := (*reflect.StringHeader)(unsafe.Pointer(&s))
	for i := 0; i < hdr.Len; i++ {
		hash *= 1099511628211
		hash ^= uint64(*(*byte)(unsafe.Pointer(hdr.Data + uintptr(i))))
	}
	return hash
}
