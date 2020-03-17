package kv

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/url"
	"os"
	"time"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal/kv/cache"
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

func calcPath2(key1, key2 string) (string, string) {
	dir := fmt.Sprintf("tmp/data/%d/%s", common.Hash32(key1)&0xff, key1)
	return dir, dir + "/" + url.PathEscape(key2) + ".txt"
}

func (m *DiskKV) SetGlobalCache(c *cache.GlobalCache) {
	m.cache = c
}

func (m *DiskKV) Get(key string) ([]byte, error) {
	nocache := false

	v, ok := m.cache.Get(key)
	if bytes.Equal(v, locker) {
		v = nil
		nocache = true
	} else if ok {
		return v, nil
	}

	if randomError > 0 && rand.Intn(randomError) == 0 {
		return nil, fmt.Errorf("1")
	}

	time.Sleep(time.Millisecond * time.Duration(rand.Intn(50)+50))

	_, fn := calcPath(key)
	_, err := os.Stat(fn)

	if err == nil {
		v, err = ioutil.ReadFile(fn)
	} else if os.IsNotExist(err) {
		err = nil
	} else {
	}

	if err == nil {
		if !nocache {
			m.cache.Add(key, v)
		}
	}

	return v, err
}

func (m *DiskKV) Set(key string, value []byte) error {
	if err := m.cache.Add(key, locker); err != nil {
		return err
	}

	if randomError > 0 && rand.Intn(randomError) == 0 {
		return fmt.Errorf("1")
	}

	dir, fn := calcPath(key)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}

	err := ioutil.WriteFile(fn, value, 0777)
	if err == nil {
		if err := m.cache.Add(key, value); err != nil {
			log.Println("KV add:", err)
		}
	}
	return err
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
