package disklru

import (
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

type File struct {
	*os.File

	closed   bool
	closeMu  sync.Mutex
	closeErr error
}

func (f *File) Close() error {
	f.closeMu.Lock()
	defer f.closeMu.Unlock()
	if f.closed {
		return f.closeErr
	}
	f.closed = true
	f.closeErr = f.File.Close()
	return f.closeErr
}

type DiskLRU struct {
	dir     string
	ptr     int
	ptrMu   sync.RWMutex
	max     int
	open    singleflight.Group
	request func(string, string) error
}

func New(dir string, maxFiles int, watchInterval time.Duration,
	request func(key, saveTo string) error) (*DiskLRU, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	d := &DiskLRU{
		dir:     dir,
		max:     maxFiles,
		request: request,
	}
	d.purger(watchInterval)
	return d, nil
}

func (d *DiskLRU) makePath(key string) [2]string {
	return [2]string{filepath.Join(d.dir, "0"+key), filepath.Join(d.dir, "1"+key)}
}

func (d *DiskLRU) GetKeyPath(key string) string {
	hot, _ := d.hotCold()
	return d.makePath(key)[hot]
}

func (d *DiskLRU) purger(watchInterval time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("DiskLRU[%s] fatal error: %v", d.dir, r)
		}
		time.AfterFunc(watchInterval, func() { d.purger(watchInterval) })
	}()

	hot, cold := d.hotCold()

	f, err := os.Open(d.dir)
	if err != nil {
		logrus.Errorf("DiskLRU[%s] failed to open: %v", d.dir, err)
		return
	}
	defer f.Close()

	tmp, err := f.Readdirnames(-1)
	if err != nil {
		logrus.Errorf("DiskLRU[%s] failed to readdir: %v", d.dir, err)
		return
	}

	var files [2][]string
	var invalids int
	for _, fn := range tmp {
		switch fn[0] {
		case '0', '1':
			i := fn[0] - '0'
			files[i] = append(files[i], fn)
		default:
			invalids++
			continue
		}
	}
	if invalids > 0 {
		logrus.Errorf("DiskLRU[%s] find %d invalid files", d.dir, invalids)
	}

	s := files[cold]
	rand.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] })

	count := len(files[0]) + len(files[1])
	deleted := 0
	for count > d.max {
		if len(files[cold]) == 0 {
			d.switchHotCold()
			logrus.Infof("DiskLRU[%s] hot cold switched, cold current size: %d", d.dir, len(files[hot]))
			break
		}
		os.Remove(filepath.Join(d.dir, files[cold][0]))
		files[cold] = files[cold][1:]
		deleted++
		count--
	}
	if deleted > 0 {
		logrus.Infof("DiskLRU[%s] purge %d cold files", d.dir, deleted)
	}
}

func (d *DiskLRU) switchHotCold() {
	d.ptrMu.RLock()
	defer d.ptrMu.RUnlock()
	d.ptr++
}

func (d *DiskLRU) hotCold() (int, int) {
	d.ptrMu.RLock()
	defer d.ptrMu.RUnlock()
	return d.ptr % 2, (d.ptr + 1) % 2
}

func (d *DiskLRU) Open(key string) (*File, error) {
	ps := d.makePath(key)
	hot, cold := d.hotCold()

	res, err, _ := d.open.Do(key, func() (interface{}, error) {
		f, err := os.Open(ps[hot])
		if err == nil {
			return f, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}

		if _, err := os.Stat(ps[cold]); err == nil {
			if err := os.Rename(ps[cold], ps[hot]); err != nil {
				return nil, err
			}
		} else {
			if err := d.request(key, ps[hot]); err != nil {
				os.Remove(ps[hot])
				return nil, err
			}
		}
		return os.Open(ps[hot])
	})

	if err != nil {
		return nil, err
	}
	return &File{File: res.(*os.File)}, nil
}
