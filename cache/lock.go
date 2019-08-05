package cache

import (
	"sync"
	"time"

	"github.com/coyove/common/sched"
)

type lk struct {
	unlockTime time.Time
	schedDeath sched.SchedKey
}

type KeyLocks struct {
	mu sync.Mutex
	l  map[string]*lk
}

func NewKeyLocks() *KeyLocks {
	return &KeyLocks{
		l: make(map[string]*lk),
	}
}

func (l *KeyLocks) Lock(v string, timeout time.Duration) sched.SchedKey {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	lv := l.l[v]

	if lv == nil || lv.unlockTime.Before(now) {
		kk := &lk{
			unlockTime: now.Add(timeout),
			schedDeath: sched.Schedule(func() {
				l.mu.Lock()
				delete(l.l, v)
				l.mu.Unlock()
			}, timeout+time.Second),
		}
		l.l[v] = kk
		return kk.schedDeath
	}
	return 0
}

func (l *KeyLocks) Unlock(v string, key sched.SchedKey) {
	l.mu.Lock()
	defer l.mu.Unlock()

	lv := l.l[v]
	if lv == nil || lv.schedDeath != key {
		return
	}

	delete(l.l, v)
	lv.schedDeath.Cancel()
}
