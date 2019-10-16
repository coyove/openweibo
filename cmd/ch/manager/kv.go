package manager

import (
	sync "sync"

	"github.com/coyove/common/lru"
	"github.com/etcd-io/bbolt"
	//sync "github.com/sasha-s/go-deadlock"
)

var randomError = true

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
		cache: lru.NewCache(128),
	}
	return r
}

func hashString(s string) (h uint16) {
	for _, r := range s {
		h = 31*h + uint16(r)
	}
	return
}

func (m *BoltKV) Lock(key string) {
	lk := &m.locks[hashString(key)]
	lk.Lock()
}

func (m *BoltKV) Unlock(key string) {
	lk := &m.locks[hashString(key)]
	lk.Unlock()
}

func (m *BoltKV) Get(keys ...string) (values map[string][]byte, err error) {
	values = map[string][]byte{}

	for i := len(keys) - 1; i >= 0; i-- {
		key := keys[i]
		x, _ := m.cache.Get(key)
		v, _ := x.([]byte)

		if len(v) > 0 {
			values[key] = v
			keys = append(keys[:i], keys[i+1:]...)
		}
	}

	if len(keys) == 0 {
		return
	}

	//time.Sleep(time.Millisecond * time.Duration(rand.Intn(50)+50))

	err = m.db.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(bkPost)
		if bk == nil {
			return nil
		}

		for _, key := range keys {
			values[key] = append([]byte{}, bk.Get([]byte(key))...)
		}

		return nil
	})
	return
}

func (m *BoltKV) Set(key string, value []byte) error {
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
