package cache

import (
	"crypto/sha1"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/coyove/common/sched"
)

var WatchInterval = time.Minute

type Cache struct {
	path    string
	maxSize int64
	factor  float64
	getter  func(k string) ([]byte, error)
}

func New(path string, maxSize int64, getter func(k string) ([]byte, error)) *Cache {
	for i := 0; i < 1024; i++ {
		dir := filepath.Join(path, strconv.Itoa(i))
		os.MkdirAll(dir, 0644)
	}

	c := &Cache{
		path:    path,
		maxSize: maxSize,
		factor:  0.9,
		getter:  getter,
	}

	c.watchCacheDir()
	return c
}

func (c *Cache) makePath(key string) string {
	k := sha1.Sum([]byte(key))
	return filepath.Join(c.path, fmt.Sprintf("%d/%x", (uint16(k[0])<<16|uint16(k[1]))/64, k[1:]))
}

func (c *Cache) watchCacheDir() {
	var totalSize int64
	var randDir = filepath.Join(c.path, strconv.Itoa(rand.Intn(1024)))

	filepath.Walk(randDir, func(path string, info os.FileInfo, err error) error {
		if info != nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	if diff := totalSize - int64(float64(c.maxSize)/1024*c.factor); diff > 0 {
		c.purge(diff)
	}

	sched.Schedule(func() { go c.watchCacheDir() }, WatchInterval)
}

func (c *Cache) purge(amount int64) {
	log.Println("[cache.purge.amount]", amount/1024/1024, "M")

	for i := 0; i < 1024; i++ {
		dir := filepath.Join(c.path, strconv.Itoa(i))
		file, err := os.Open(dir)
		if err != nil {
			log.Println("[cache.purge]", err, dir)
			continue
		}

		names, err := file.Readdirnames(0)
		if err != nil {
			log.Println("[cache.purge]", err, dir)
			continue
		}

		log.Println("[cache.purge.names]", len(names))

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

		log.Println("[cache.purge.ok]", dir)
	}
}

func (c *Cache) Get(key string) ([]byte, error) {
	k := c.makePath(key)
}
