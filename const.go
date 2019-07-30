package node

import "errors"

var (
	ErrKeyNotFound        = errors.New("key not found")
	ErrServiceUnavailable = errors.New("service unavailable")
)
