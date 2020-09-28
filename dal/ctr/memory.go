package ctr

import "sync"

type MemBack struct {
	sync.Mutex
	m map[int64]int64
}

func (m *MemBack) Set(k int64, v int64) int64 {
	m.Lock()
	old := m.m[k]
	m.m[k] = v
	m.Unlock()
	return old
}

func (m *MemBack) Put(k int64, v int64) (int64, bool) {
	m.Lock()
	defer m.Unlock()

	if v2, ok := m.m[k]; ok {
		return v2, false
	}
	m.m[k] = v
	return v, true
}
