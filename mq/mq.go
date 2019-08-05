package mq

import (
	"encoding/binary"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/etcd-io/bbolt"
)

var (
	bkName        = []byte("queue")
	bkKVName      = []byte("kv")
	prefetch      = 64
	ErrEmptyQueue = errors.New("empty queue")
)

type SimpleMessageQueue struct {
	mu       sync.Mutex
	db       *bbolt.DB
	f        *os.File
	kvs      [][2][]byte
	consumed [][]byte
	counter  uint32
}

func New(path string) (*SimpleMessageQueue, error) {
	os.MkdirAll(filepath.Dir(path), 0700)

	db, err := bbolt.Open(path, 0700, nil)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".con", os.O_CREATE|os.O_RDWR, 0700)
	if err != nil {
		return nil, err
	}

	if err := db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bkKVName); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(bkName)
		return err
	}); err != nil {
		return nil, err
	}

	mq := &SimpleMessageQueue{
		db: db,
		f:  f,
	}
	if err := mq.checkConFile(); err != nil {
		return nil, err
	}
	return mq, nil
}

func (mq *SimpleMessageQueue) Close() error {
	mq.f.Close()
	return mq.db.Close()
}

func (mq *SimpleMessageQueue) PushBack(value ...[]byte) error {
	return mq.db.Update(func(tx *bbolt.Tx) error {
		key := [8]byte{}
		idx := uint16(rand.Uint64())
		for _, value := range value {
			idx++
			binary.BigEndian.PutUint32(key[:4], uint32(time.Now().Unix()))
			binary.BigEndian.PutUint16(key[4:], uint16(atomic.AddUint32(&mq.counter, 1)))
			binary.BigEndian.PutUint16(key[6:], idx)
			if err := tx.Bucket(bkName).Put(key[:], value); err != nil {
				return err
			}
		}
		return nil
	})
}

func (mq *SimpleMessageQueue) PopFront() (*Message, error) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	for {
		if len(mq.kvs) > 0 {
			k, v := mq.kvs[0][0], mq.kvs[0][1]
			mq.kvs = mq.kvs[1:]

			if err := mq.addToConsumed(k); err != nil {
				return nil, err
			}

			return &Message{
				q:     mq,
				key:   k,
				Value: v,
				Time:  time.Unix(int64(binary.BigEndian.Uint32(k)), 0),
			}, nil
		}

		// Delete consumed keys first
		if err := mq.db.Update(func(tx *bbolt.Tx) error {
			bk := tx.Bucket(bkName)
			// log.Println(len(mq.consumed), bk.Stats().KeyN)
			for _, key := range mq.consumed {
				if err := bk.Delete(key); err != nil {
					return err
				}
			}
			// log.Println(bk.Stats().KeyN)
			return mq.clearConsumed()
		}); err != nil {
			return nil, err
		}

		// Iterate the remaining keys
		if err := mq.db.View(func(tx *bbolt.Tx) error {
			bk := tx.Bucket(bkName)
			c := bk.Cursor()
			for k, v := c.First(); len(k) == 8; k, v = c.Next() {
				mq.kvs = append(mq.kvs, [2][]byte{
					dupBytes(k),
					dupBytes(v),
				})
				if len(mq.kvs) >= prefetch {
					break
				}
			}

			if len(mq.kvs) == 0 {
				return ErrEmptyQueue
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
}
