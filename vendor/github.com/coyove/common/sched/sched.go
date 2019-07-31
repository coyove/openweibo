package sched

import (
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	//sync "github.com/sasha-s/go-deadlock"
)

var Verbose = true

type notifier struct {
	deadline int64
	callback func()
}

var timeoutWheel struct {
	secmin [60][60]struct {
		sync.Mutex
		counter uint64
		list    map[SchedKey]*notifier
	}
	maxsyncfires int
}

func init() {
	go func() {
		for t := range time.Tick(time.Second) {
			s, m, now := t.Second(), t.Minute(), t.Unix()

			syncNotifiers := make([]*notifier, 0, 16)

			for i := 0; i < 2; i++ {
				ts := &timeoutWheel.secmin[s][m]
				ts.Lock()
				for k, n := range ts.list {
					if n.deadline/1e9 > now {
						continue
					}

					delete(ts.list, k)
					syncNotifiers = append(syncNotifiers, n)
				}
				ts.Unlock()

				// Dial back 1 second to check if any callbacks which should time out at "this second"
				// are added to the "previous second" because of clock precision
				t = time.Unix(now-1, 0)
				s, m = t.Second(), t.Minute()
			}

			sort.Slice(syncNotifiers, func(i, j int) bool {
				return syncNotifiers[i].deadline < syncNotifiers[j].deadline
			})

			for _, n := range syncNotifiers {
				if diff, idiff := n.deadline/1e6-time.Now().UnixNano()/1e6, now*1e3-n.deadline/1e6; diff > 120 && idiff < 880 {
					time.Sleep(time.Duration(diff) * time.Millisecond)
				}
				n.callback()
			}

			if len(syncNotifiers) > timeoutWheel.maxsyncfires {
				timeoutWheel.maxsyncfires = len(syncNotifiers)
			}

			if Verbose {
				log.Println("fires:", len(syncNotifiers), "max:", timeoutWheel.maxsyncfires)
			}
		}
	}()
}

type SchedKey uint64

func Schedule(callback func(), deadlineOrTimeout interface{}) SchedKey {
	deadline := time.Now()
	now := deadline.Unix()

	switch d := deadlineOrTimeout.(type) {
	case time.Time:
		deadline = d
	case time.Duration:
		deadline = deadline.Add(d)
	default:
		panic("invalid deadline(time.Time) or timeout(time.Duration) value")
	}

	dead := deadline.Unix()

	if now > dead {
		// timed out already
		return 0
	} else if now == dead {
		callback()
		return 0
	}

	s, m := deadline.Second(), deadline.Minute()
	ts := &timeoutWheel.secmin[s][m]
	ts.Lock()

	ts.counter++

	// sec (6bit) | min (6bit) | counter (52bit)
	// key will never be 0
	key := SchedKey(uint64(s+1)<<58 | uint64(m+1)<<52 | (ts.counter & 0xfffffffffffff))

	if ts.list == nil {
		ts.list = map[SchedKey]*notifier{}
	}

	ts.list[key] = &notifier{
		deadline: deadline.UnixNano(),
		callback: callback,
	}

	ts.Unlock()
	return key
}

func (key SchedKey) Cancel() (oldcallback func()) {
	s := int(key>>58) - 1
	m := int(key<<6>>58) - 1
	if s < 0 || s > 59 || m < 0 || m > 59 {
		return
	}
	ts := &timeoutWheel.secmin[s][m]
	ts.Lock()
	if ts.list != nil {
		if n := ts.list[key]; n != nil {
			oldcallback = n.callback
		}
		delete(ts.list, key)
	}
	ts.Unlock()
	return
}

// If callback is nil, the old one will be reused
func (key *SchedKey) Reschedule(callback func(), deadlineOrTimeout interface{}) {
RETRY:
	old := atomic.LoadUint64((*uint64)(key))
	f := SchedKey(old).Cancel()
	if f == nil && callback == nil {
		// The key has already been canceled, there is no way to reschedule it
		// if no valid callback is provided
		return
	}
	if callback == nil {
		callback = f
	}
	n := Schedule(callback, deadlineOrTimeout)
	if atomic.CompareAndSwapUint64((*uint64)(key), old, uint64(n)) {
		return
	}

	n.Cancel()
	goto RETRY
}
