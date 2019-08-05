package mq

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/etcd-io/bbolt"
)

type Message struct {
	q        *SimpleMessageQueue
	returned uintptr
	key      []byte
	Value    []byte
	Time     time.Time
}

func (m *Message) String() string {
	return fmt.Sprintf("<Message-%x:%s,Time:%v>", m.Index(), string(m.Value), m.Time)
}

func (m *Message) Index() uint64 {
	return binary.BigEndian.Uint64(m.key)
}

func (m *Message) PutBack() error {
	if !atomic.CompareAndSwapUintptr(&m.returned, 0, 1) {
		panic("double put back")
	}

	mq := m.q
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if err := mq.delConsumed(m.key); err != nil {
		return err
	}
	if err := mq.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bkName).Put(m.key, m.Value)
	}); err != nil {
		return err
	}

	// the message we put back will be the first to be popped in next round
	mq.kvs = append(mq.kvs, [2][]byte{})
	copy(mq.kvs[1:], mq.kvs)
	mq.kvs[0] = [2][]byte{m.key, m.Value}
	return nil
}

func dupBytes(p []byte) []byte {
	if p == nil {
		return nil
	}
	p2 := make([]byte, len(p))
	copy(p2, p)
	return p2
}
