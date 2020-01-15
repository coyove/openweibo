package kv

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal/cache"
	//sync "github.com/sasha-s/go-deadlock"
)

type DiskKV struct {
	cache *cache.GlobalCache
}

func NewDiskKV() *DiskKV {
	err := os.MkdirAll("tmp/data/", 0777)
	if err != nil {
		panic(err)
	}

	r := &DiskKV{}
	return r
}

func calcPath(key string) (string, string) {
	dir := fmt.Sprintf("tmp/data/%d", common.Hash32(key)&0xff)
	return dir, dir + "/" + url.PathEscape(key) + ".txt"
}

func (m *DiskKV) SetGlobalCache(c *cache.GlobalCache) {
	m.cache = c
}

func (m *DiskKV) Get(key string) ([]byte, error) {
	v, ok := m.cache.Get(key)

	if ok {
		return v, nil
	}

	if randomError > 0 && rand.Intn(randomError) == 0 {
		return nil, fmt.Errorf("1")
	}

	// time.Sleep(time.Millisecond * time.Duration(rand.Intn(50)+50))
	_, fn := calcPath(key)
	_, err := os.Stat(fn)

	if err == nil {
		v, err = ioutil.ReadFile(fn)
	} else if os.IsNotExist(err) {
		err = nil
	} else {
	}

	if err == nil {
		m.cache.Add(key, v)
	}

	return v, err
}

func (m *DiskKV) Set(key string, value []byte) error {
	m.cache.Remove(key)

	if randomError > 0 && rand.Intn(randomError) == 0 {
		return fmt.Errorf("1")
	}

	dir, fn := calcPath(key)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}

	return ioutil.WriteFile(fn, value, 0777)
}

func (m *DiskKV) Delete(key string) error {
	panic(key)
	// m.cache.Remove(key)

	// return m.db.Update(func(tx *bbolt.Tx) error {
	// 	bk, err := tx.CreateBucketIfNotExists(bkPost)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	return bk.Delete([]byte(key))
	// })
}
