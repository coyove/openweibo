package cache

import (
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coyove/common/sched"
)

var WatchInterval = time.Minute

type Cache struct {
	mu      *KeyLocks
	path    string
	maxSize int64
	factor  float64
	getter  func(k string) ([]byte, error)

	survey struct {
		hits, misses int64
	}
}

func New(path string, maxSize int64, getter func(k string) ([]byte, error)) *Cache {
	for i := 0; i < 1024; i++ {
		dir := filepath.Join(path, strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0777); err != nil {
			panic(err)
		}
	}

	c := &Cache{
		path:    path,
		maxSize: maxSize,
		factor:  0.9,
		getter:  getter,
		mu:      NewKeyLocks(),
	}

	c.watchCacheDir()
	return c
}

func (c *Cache) makePath(key string) string {
	k := sha1.Sum([]byte(key))
	idx := (uint16(k[0])<<8 | uint16(k[1])) / 64
	return filepath.Join(c.path, fmt.Sprintf("%d/%x", idx, k[1:]))
}

func (c *Cache) watchCacheDir() {
	var totalSize int64
	var r = rand.Intn(1024)
	var randDir = filepath.Join(c.path, strconv.Itoa(r))

	filepath.Walk(randDir, func(path string, info os.FileInfo, err error) error {
		if info != nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	log.Println("[cache.survey]", randDir, "size:", totalSize, "b")

	if diff := totalSize - int64(float64(c.maxSize)/1024*c.factor); diff > 0 {
		c.purge(diff)
	}

	sched.Schedule(func() { go c.watchCacheDir() }, WatchInterval)
}

func (c *Cache) purge(amount int64) {
	log.Println("[cache.purge.amount]", amount, "b")

	start := time.Now()
	totalNames := 0

	for i := 0; i < 1024; i++ {
		dir := filepath.Join(c.path, strconv.Itoa(i))
		file, err := os.Open(dir)
		if err != nil {
			log.Println("[cache.purge]", err, dir)
			continue
		}

		names, err := file.Readdirnames(0)
		file.Close()

		if err != nil {
			log.Println("[cache.purge]", err, dir)
			continue
		}

		totalNames += len(names)

		for x := amount; x > 0 && len(names) > 0; {
			idx := rand.Intn(len(names))
			names[idx], names[0] = names[0], names[idx]
			name := names[0]
			names = names[1:]
			path := filepath.Join(dir, name)

			info, err := os.Stat(path)
			if err != nil {
				log.Println("[cache.purge]", err, path)
				continue
			}
			if err := os.Remove(path); err != nil {
				log.Println("[cache.purge]", err, path)
				continue
			}
			x -= info.Size()
		}
	}

	log.Println("[cache.purge.ok]", time.Since(start).Nanoseconds()/1e6, "ms,", totalNames, "names")
}

func (c *Cache) Fetch(key string) ([]byte, error) {
	k := c.makePath(key)

	buf, err := ioutil.ReadFile(k)
	if err == nil {
		atomic.AddInt64(&c.survey.hits, 1)
		return buf, nil
	}

	lockkey := c.mu.Lock(k, time.Second*2)

	if lockkey != 0 {
		defer c.mu.Unlock(k, lockkey)
	} else {
		time.Sleep(time.Second * 2)
		buf, err := ioutil.ReadFile(k)
		if err == nil {
			atomic.AddInt64(&c.survey.hits, 1)
			return buf, nil
		}
	}

	atomic.AddInt64(&c.survey.misses, 1)
	buf, err = c.getter(key)
	if err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(k, buf, 0777); err != nil {
		log.Println("[cache.get]", err)
	}
	return buf, nil
}

func (c *Cache) HitRate() float64 {
	return float64(c.survey.hits) / float64(c.survey.hits+c.survey.misses+1)
}
