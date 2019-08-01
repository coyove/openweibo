package chbolt

import (
	"github.com/boltdb/bolt"
	"github.com/coyove/ch/driver"
)

func NewNode(name, path string, weight int64) *driver.Node {
	db, err := bolt.Open(path, 0644, nil)
	if err != nil {
		panic(err)
	}

	return &driver.Node{
		KV: &Storage{
			db:   db,
			name: name,
		},
		Name:   name,
		Weight: weight,
	}
}

type Storage struct {
	name string
	db   *bolt.DB
}

func (s *Storage) DB() *bolt.DB {
	return s.db
}

func (s *Storage) Put(k string, v []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bk, err := tx.CreateBucketIfNotExists([]byte(s.name))
		if err != nil {
			return err
		}
		return bk.Put([]byte(k), v)
	})
}

func (s *Storage) Get(k string) ([]byte, error) {
	var v []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket([]byte(s.name))
		if bk == nil {
			return driver.ErrKeyNotFound
		}
		v = bk.Get([]byte(k))
		if v == nil {
			return driver.ErrKeyNotFound
		}
		return nil
	})
	return v, err
}

func (s *Storage) Delete(k string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bk, err := tx.CreateBucketIfNotExists([]byte(s.name))
		if err != nil {
			return err
		}
		return bk.Delete([]byte(k))
	})
}

func (s *Storage) Stat() driver.Stat {
	return driver.Stat{}
}
