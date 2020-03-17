package kv

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
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
	dir := fmt.Sprintf("tmp/data/%d/%s", common.Hash32(key1)&0xff, url.PathEscape(key1))
	return dir, dir + "/" + key2
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

func (m *DiskKV) Get2(key1, key2 string) ([]byte, error) {
	nocache := false

	v, ok := m.cache.Get(key1 + "..." + key2)
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

	_, fn := calcPath2(key1, key2)
	_, err := os.Stat(fn)

	if err == nil {
		v, err = ioutil.ReadFile(fn)
	} else if os.IsNotExist(err) {
		err = nil
	} else {
	}

	if err == nil {
		if !nocache {
			m.cache.Add(key1+"..."+key2, v)
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

func (m *DiskKV) Set2(key1, key2 string, value []byte) error {
	if err := m.cache.Add(key1+"..."+key2, locker); err != nil {
		return err
	}

	if randomError > 0 && rand.Intn(randomError) == 0 {
		return fmt.Errorf("1")
	}

	dir, fn := calcPath2(key1, key2)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}

	err := ioutil.WriteFile(fn, value, 0777)
	if err == nil {
		if err := m.cache.Add(key1+"..."+key2, value); err != nil {
			log.Println("KV add:", err)
		}
	}
	return err
}

func (m *DiskKV) Range(key, start string, n int) ([][]byte, string, error) {
	dir, _ := calcPath2(key, "")
	files, _ := ioutil.ReadDir(dir)

	starti := len(files) - 1
	if start != "" {
		for i, f := range files {
			if f.Name() == start {
				starti = i - 1
				break
			}
		}
	}

	res, next := [][]byte{}, ""
	for i := starti; i >= 0; i-- {
		p, _ := ioutil.ReadFile(filepath.Join(dir, files[i].Name()))
		res = append(res, p)

		if len(res) >= n+1 {
			next = files[i].Name()
			res = res[:n]
			break
		}
	}

	return res, next, nil
}
