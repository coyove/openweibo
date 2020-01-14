package kv

import (
	"fmt"
	"math/rand"
	sync "sync"
	"time"

	"github.com/coyove/common/lru"
	"github.com/etcd-io/bbolt"
	//sync "github.com/sasha-s/go-deadlock"
)

var bkPost = []byte("post")

type BoltKV struct {
	cache *lru.Cache
	db    *bbolt.DB
	locks [65536]sync.Mutex
}

func NewBoltKV(path string) *BoltKV {
	db, err := bbolt.Open(path, 0700, &bbolt.Options{FreelistType: bbolt.FreelistMapType})
	if err != nil {
		panic(err)
	}

	r := &BoltKV{
		db:    db,
		cache: lru.NewCache(CacheSize),
	}
	return r
}

func (m *BoltKV) ResetCache() {
}

func (m *BoltKV) Lock(key string) {
	lk := &m.locks[hashString(key)]
	lk.Lock()
}

func (m *BoltKV) Unlock(key string) {
	lk := &m.locks[hashString(key)]
	lk.Unlock()
}

func (m *BoltKV) Get(key string) ([]byte, error) {
	x, _ := m.cache.Get(key)
	v, _ := x.([]byte)

	if len(v) > 0 {
		return v, nil
	}

	if randomError > 0 && rand.Intn(randomError) == 0 {
		return nil, fmt.Errorf("1")
	}

	time.Sleep(time.Millisecond * time.Duration(rand.Intn(50)+50))

	err := m.db.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(bkPost)
		if bk == nil {
			return nil
		}

		v = append([]byte{}, bk.Get([]byte(key))...)
		return nil
	})

	if len(v) > 0 {
		m.cache.Add(key, v)
	}

	return v, err
}

func (m *BoltKV) Set(key string, value []byte) error {
	if randomError > 0 && rand.Intn(randomError) == 0 {
		return fmt.Errorf("1")
	}

	return m.db.Update(func(tx *bbolt.Tx) error {
		bk, err := tx.CreateBucketIfNotExists(bkPost)
		if err != nil {
			return err
		}

		m.cache.Remove(key)
		if err := bk.Put([]byte(key), value); err != nil {
			return err
		}
		m.cache.Add(key, value)
		return nil
	})
}

func (m *BoltKV) Delete(key string) error {
	m.cache.Remove(key)

	return m.db.Update(func(tx *bbolt.Tx) error {
		bk, err := tx.CreateBucketIfNotExists(bkPost)
		if err != nil {
			return err
		}
		return bk.Delete([]byte(key))
	})
}
