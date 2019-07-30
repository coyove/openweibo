package memory

import (
	"sync"

	"github.com/coyove/ch/driver"
)

type Storage struct {
	kv sync.Map
}

func (s *Storage) Put(k string, v []byte) error {
	s.kv.Store(k, v)
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
	return nil
}

func (s *Storage) Stat() *driver.Stat {
	return &driver.Stat{}
}
