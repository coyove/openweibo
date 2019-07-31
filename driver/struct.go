package driver

import (
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrKeyNotFound  = errors.New("key not found")
	ErrFullCapacity = errors.New("storage: no space left")
	ErrThrottled    = errors.New("storage: resource temporarily throttled")
)

type Stat struct {
	TotalBytes     int64
	AvailableBytes int64
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
