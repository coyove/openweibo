package mq

import (
	"encoding/binary"
	"time"

	"github.com/etcd-io/bbolt"
)

func (mq *SimpleMessageQueue) View(cursor uint64, n int) ([]*Message, uint64, error) {
	var (
		values []*Message
		next   uint64
		m      = map[uint64]bool{}
	)

	mq.mu.Lock()
	for _, c := range mq.consumed {
		m[binary.BigEndian.Uint64(c)] = true
	}
	mq.mu.Unlock()

	err := mq.db.View(func(tx *bbolt.Tx) error {
		cbuf := make([]byte, 8)
		binary.BigEndian.PutUint64(cbuf, cursor)

		c := tx.Bucket(bkName).Cursor()
		for k, v := c.Seek(cbuf); len(k) == 8; k, v = c.Next() {
			if m[binary.BigEndian.Uint64(k)] {
				continue
			}
			values = append(values, &Message{
				q:     mq,
				Value: dupBytes(v),
				key:   k,
				Time:  time.Unix(int64(binary.BigEndian.Uint32(k)), 0),
			})
			if len(values) >= n {
				break
			}
		}

		if len(values) == 0 {
			return ErrEmptyQueue
		}

		next = values[len(values)-1].Index() + 1
		return nil
	})

	return values, next, err
}

func (mq *SimpleMessageQueue) Len() int {
	var l int
	mq.db.View(func(tx *bbolt.Tx) error {
		l = tx.Bucket(bkName).Stats().KeyN
		return nil
	})
	return l - len(mq.consumed)
}

func (mq *SimpleMessageQueue) GetK(k string) ([]byte, error) {
	var value []byte
	err := mq.db.View(func(tx *bbolt.Tx) error {
		value = dupBytes(tx.Bucket(bkKVName).Get([]byte(k)))
		return nil
	})
	return value, err
}

func (mq *SimpleMessageQueue) PutK(k string, v []byte) error {
	return mq.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bkKVName).Put([]byte(k), v)
	})
}

func (mq *SimpleMessageQueue) DeleteK(k string) error {
	return mq.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bkKVName).Delete([]byte(k))
	})
}
