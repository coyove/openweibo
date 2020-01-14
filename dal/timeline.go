package dal

import (
	"sort"
	"sync"
)

type tlCache struct {
	mu  sync.RWMutex
	max int
	u   []string
}

func (tl *tlCache) add(cursor string) {
	if cursor == "" {
		return
	}

	tl.mu.Lock()
	defer tl.mu.Unlock()

	i := sort.SearchStrings(tl.u, cursor)

	if i < len(tl.u) && tl.u[i] == cursor {
		return
	}

	tl.u = append(tl.u, "")
	copy(tl.u[i+1:], tl.u[i:])
	tl.u[i] = cursor

	if len(tl.u) > tl.max {
		tl.u = tl.u[len(tl.u)-tl.max:]
	}
}

func (tl *tlCache) del(cursor string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	i := sort.SearchStrings(tl.u, cursor)

	if i < len(tl.u) && tl.u[i] == cursor {
		tl.u = append(tl.u[:i], tl.u[i+1:]...)
	}
}

func (tl *tlCache) findPrev(cursor string) string {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	i := sort.SearchStrings(tl.u, cursor)

	if i >= len(tl.u) {
		return ""
	}

	if tl.u[i] != cursor {
		return tl.u[i]
	}

	if i < len(tl.u)-1 {
		return tl.u[i+1]
	}

	return ""
}
