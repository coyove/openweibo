package driver

import "errors"

var (
	ErrKeyNotFound        = errors.New("key not found")
	ErrServiceUnavailable = errors.New("service unavailable")
)

type Stat struct {
	TotalBytes     int64
	AvailableBytes int64
	Ping           int64
}

type KV interface {
	Put(k string, v []byte) error
	Get(k string) ([]byte, error)
	Delete(k string) error
	Stat() *Stat
}
