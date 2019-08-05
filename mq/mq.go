package mq

import (
	"encoding/binary"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/etcd-io/bbolt"
)

var (
	bkName        = []byte("queue")
	ErrEmptyQueue = errors.New("empty queue")
)

type SimpleMessageQueue struct {
	mu       sync.Mutex
	db       *bbolt.DB
	kvs      [][2][]byte
	consumed [][]byte
	counter  uint32
}

func New(path string) (*SimpleMessageQueue, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}

	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bkName)
		return err
	}); err != nil {
		return nil, err
	}

	mq := &SimpleMessageQueue{
		db: db,
	}
	return mq, nil
}

func (mq *SimpleMessageQueue) Close() error {
	return mq.db.Close()
}

func (mq *SimpleMessageQueue) PushBack(value []byte) error {
	return mq.db.Update(func(tx *bbolt.Tx) error {
		key := [8]byte{}
		binary.BigEndian.PutUint32(key[:4], uint32(time.Now().Unix()))
		binary.BigEndian.PutUint16(key[4:], uint16(atomic.AddUint32(&mq.counter, 1)))
		binary.BigEndian.PutUint16(key[6:], uint16(rand.Uint64()))
		return tx.Bucket(bkName).Put(key[:], value)
	})
}

func (mq *SimpleMessageQueue) PopFront() ([]byte, time.Time, error) {
	key, value, err := mq.first()
	if err != nil {
		return nil, time.Time{}, err
	}
	mq.consumed = append(mq.consumed, key)
	return value, time.Unix(int64(binary.BigEndian.Uint32(key)), 0), nil
}

func (mq *SimpleMessageQueue) first() ([]byte, []byte, error) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	for {
		if len(mq.kvs) > 0 {
			k := mq.kvs[0][0]
			v := mq.kvs[0][1]
			mq.kvs = mq.kvs[1:]
			return k, v, nil
		}

		if err := mq.db.Update(func(tx *bbolt.Tx) error {
			bk := tx.Bucket(bkName)
			// Delete consumed keys first
			for _, key := range mq.consumed {
				if err := bk.Delete(key); err != nil {
					return err
				}
			}
			mq.consumed = mq.consumed[:0]
			// Iterate the remaining keys
			c := bk.Cursor()
			for k, v := c.First(); len(k) == 8; k, v = c.Next() {
				mq.kvs = append(mq.kvs, [2][]byte{
					dupBytes(k),
					dupBytes(v),
				})
				if len(mq.kvs) >= 64 {
					break
				}
			}

			if len(mq.kvs) == 0 {
				return ErrEmptyQueue
			}
			return nil
		}); err != nil {
			return nil, nil, err
		}
	}
}

func dupBytes(p []byte) []byte {
	p2 := make([]byte, len(p))
	copy(p2, p)
	return p2
}

func (mq *SimpleMessageQueue) FirstN(n int) ([][]byte, []time.Time, error) {
	var values [][]byte
	var times []time.Time

	err := mq.db.View(func(tx *bbolt.Tx) error {
		c := tx.Bucket(bkName).Cursor()
		for k, v := c.First(); len(k) == 8; k, v = c.Next() {
			values = append(values, dupBytes(v))
			times = append(times, time.Unix(int64(binary.BigEndian.Uint32(k)), 0))
			if len(values) >= n {
				break
			}
		}
		if len(values) == 0 {
			return ErrEmptyQueue
		}
		return nil
	})

	return values, times, err
}
