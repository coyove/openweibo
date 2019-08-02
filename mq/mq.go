package mq

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger"
)

var (
	counter uint32
)

type SimpleMessageQueue struct {
	mu   sync.Mutex
	db   *badger.DB
	keys []string
	gc   *time.Ticker
}

func New(path string) (*SimpleMessageQueue, error) {
	opts := badger.DefaultOptions(path)
	db, err := badger.Open(opts)

	if err != nil {
		return nil, err
	}

	mq := &SimpleMessageQueue{
		db: db,
		gc: time.NewTicker(time.Second * 10),
	}

	go func() {
		for range mq.gc.C {
			for {
				if err := db.RunValueLogGC(0.7); err != nil {
					log.Println("[sms.gc]", err)
					break
				}
			}
		}
		log.Println("[sms.gc] closed")
	}()
	return mq, nil
}

func (mq *SimpleMessageQueue) Close() error {
	mq.gc.Stop()
	return mq.db.Close()
}

func (mq *SimpleMessageQueue) PushBack(value []byte) error {
	return mq.db.Update(func(tx *badger.Txn) error {
		key := [8]byte{}
		binary.BigEndian.PutUint32(key[:4], uint32(time.Now().Unix()))
		binary.BigEndian.PutUint32(key[4:], atomic.AddUint32(&counter, 1))
		return tx.Set(key[:], value)
	})
}

func (mq *SimpleMessageQueue) PopFront() ([]byte, error) {
	mq.mu.Lock()
	if len(mq.keys) == 0 {
		if err := mq.refill(); err != nil {
			mq.mu.Unlock()
			return nil, err
		}
	}
	k := mq.keys[0]
	mq.keys = mq.keys[1:]
	mq.mu.Unlock()

	var value []byte
	err := mq.db.Update(func(tx *badger.Txn) error {
		item, err := tx.Get([]byte(k))
		if err != nil {
			return err
		}

		if err := item.Value(func(v []byte) error {
			value = v
			return nil
		}); err != nil {
			return err
		}
		return tx.Delete([]byte(k))
	})

	if err != nil {
		log.Println("[sms.pop]", err)
		mq.mu.Lock()
		// Misorder, but okay
		mq.keys = append(mq.keys, k)
		mq.mu.Unlock()
	}

	return value, err
}

func (mq *SimpleMessageQueue) refill() error {
	err := mq.db.View(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := tx.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			mq.keys = append(mq.keys, string(item.Key()))
			if len(mq.keys) >= 100 {
				break
			}
		}

		log.Println("[sms.refill]", len(mq.keys), "keys")
		return nil
	})

	if len(mq.keys) == 0 {
		return fmt.Errorf("empty queue")
	}
	return err
}
