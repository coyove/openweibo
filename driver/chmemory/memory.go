package chmemory

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/coyove/ch/driver"
	"github.com/coyove/common/sched"
)

func NewNode(name string, weight int64) *driver.Node {
	n := &driver.Node{
		KV: &Storage{
			weight:  weight,
			Offline: false,
		},
		Name:   name,
		Weight: weight,
	}
	n.KV.(*Storage).checkSpace()
	return n
}

type Storage struct {
	kv      sync.Map
	count   int64
	weight  int64
	used    int64
	Offline bool
}

func (s *Storage) Put(k string, v []byte) error {
	s.kv.Store(k, v)
	atomic.AddInt64(&s.count, 1)
	return nil
}

func (s *Storage) Get(k string) ([]byte, error) {
	v, ok := s.kv.Load(k)
	if ok {
		return v.([]byte), nil
	}
	return nil, driver.ErrKeyNotFound
}

func (s *Storage) Delete(k string) error {
	s.kv.Delete(k)
	atomic.AddInt64(&s.count, -1)
	return nil
}

func (s *Storage) Stat() driver.Stat {
	return driver.Stat{
		ObjectCount: s.count,
	}
}

func (s *Storage) Space() (bool, int64, int64) {
	return s.Offline, s.weight, s.used
}

func (s *Storage) checkSpace() {
	var used int64
	s.kv.Range(func(k, v interface{}) bool {
		used += int64(len(v.([]byte)))
		return true
	})
	s.used = used
	sched.Schedule(func() {
		go s.checkSpace()
	}, time.Second)
}
