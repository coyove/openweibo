package ctr

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync/atomic"
	"time"
)

var (
	Logger   = log.New(os.Stderr, "", log.LstdFlags)
	LongRace = time.Second / 2
	survey   struct {
		totalDecls int64
		totalTries int64
	}
)

// Backend: for any errors, the implementation should panic
type Backend interface {
	// Set updates key = value, and returns the old value
	Set(key int64, newValue int64) (oldValue int64)
	// Put returns (newValue, true) when key does not exist, or (alreadyStoredValue, false)
	Put(key int64, newValue int64) (oldValue int64, succeed bool)
}

type Counter struct {
	bk      Backend
	stride  int64
	lastErr error
	in      chan int64
}

func New(stride int64, backend Backend) *Counter {
	if stride < 10 {
		stride = 10
	}
	c := &Counter{
		bk:     backend,
		stride: stride,
		in:     make(chan int64, stride/2),
	}
	go func() {
		for {
			c.declareRange()
			if c.lastErr != nil {
				Logger.Println("counter: wait due to last error:", c.lastErr)
				time.Sleep(time.Second)
			}
		}
	}()
	return c
}

func (c *Counter) String() string {
	if c.lastErr != nil {
		return c.lastErr.Error()
	}
	return fmt.Sprintf("<q=%d>", len(c.in))
}

func (c *Counter) declareRange() {
	defer func() {
		if r := recover(); r != nil {
			c.lastErr = fmt.Errorf("fatal: %v", r)
			c.in <- -1
		}
	}()

	cp, _ := c.bk.Put(-1, 0)

	atomic.AddInt64(&survey.totalDecls, 1)
	for start := time.Now(); ; {
		atomic.AddInt64(&survey.totalTries, 1)

		end, ok := c.bk.Put(cp, cp+c.stride)
		if ok {
			break
		}

		if time.Since(start) > LongRace {
			Logger.Println("counter: race too long")
			time.Sleep(time.Second + time.Duration(rand.Intn(500))*time.Millisecond)
		}

		cp = end
		staticCP, _ := c.bk.Put(-1, 0)
		if cp < staticCP {
			Logger.Printf("counter: race fall behind: %d => %d", cp, staticCP)
			cp = staticCP
		}
	}

	// Actually we don't care if we override a higher checkpoint with a lower one
	// as long as they are not "overly" far apart.
	// The more distance between the real checkpoint and '.cp', the longer it takes to call "Get()'
	// because Get() has to iterate till it finds the real checkpoint
	end := cp + c.stride
	oldEnd := c.bk.Set(-1, end)
	if oldEnd > end {
		Logger.Printf("counter: drifted checkpoint %d => %d, don't panic\n", oldEnd, end)
	}

	c.lastErr = nil
	for i := cp; i < end; i++ {
		c.in <- i
	}
}

func (c *Counter) Get() (int64, error) {
	select {
	case v := <-c.in:
		if v == -1 {
			if c.lastErr != nil {
				return 0, c.lastErr
			}
			Logger.Println("counter: last error resolved, continue")
			return c.Get()
		}
		return v, nil
	}
}
