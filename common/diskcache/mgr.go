package diskcache

import (
	"crypto/sha1"
	"encoding/binary"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/coyove/common/lru"
)

type DiskCache struct {
	root      string
	slice     int
	sliceSize int64
	names     *lru.Cache
}

func New(root string, slice int, maxSize int64) (*DiskCache, error) {
	os.MkdirAll(root, 0777)
	c := &DiskCache{
		root:      root,
		slice:     slice,
		sliceSize: maxSize / int64(slice),
		names:     lru.NewCache(int64(slice)),
	}
	go c.worker()
	return c, nil
}

func (c *DiskCache) Write(name string, data []byte) error {
	dir, fn := c.getPath(name)
	fn += "_" + strconv.Itoa(len(data))
	c.names.Add(name, fn)
	os.MkdirAll(dir, 0777)
	return ioutil.WriteFile(fn, data, 0777)
}

func (c *DiskCache) Read(name string) ([]byte, error) {
	_, fn := c.getPath(name)
	return ioutil.ReadFile(fn)
}

func (c *DiskCache) getPath(name string) (string, string) {
	x := sha1.Sum([]byte(name))
	sIdx := int(binary.BigEndian.Uint64(x[:8])) % c.slice
	dir := filepath.Join(c.root, strconv.Itoa(sIdx))
	return dir, filepath.Join(dir, name)
}

func (c *DiskCache) worker() {
	for i := 0; ; i++ {
		path := filepath.Join(c.root, strconv.Itoa(i%c.slice))
		if shrinkDirSize(path, c.sliceSize) {
			time.Sleep(time.Second * 10)
		}
	}
}

func shrinkDirSize(dir string, sz int64) (sleep bool) {
	d, err := ioutil.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Println("shrinkDirSize:", dir, ":", err)
		}
		return true
	}

	var totalSize int64
	for _, f := range d {
		totalSize += f.Size()
	}

	if totalSize <= sz {
		return true
	}

	log.Println("shrinkDirSize:", dir, totalSize, "<=>", sz)
	for totalSize > sz {
		i := rand.Intn(len(d))
		totalSize -= d[i].Size()

		if err := os.Remove(filepath.Join(dir, d[i].Name())); err != nil {
			log.Println("shrinkDirSize:", dir, d[i].Name(), ":", err)
		}

		d = append(d[:i], d[i+1:]...)
	}

	return false
}
