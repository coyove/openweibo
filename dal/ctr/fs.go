package ctr

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
)

type FSBack struct {
	sync.Mutex
	dir string
}

func (m *FSBack) Set(k int64, v int64) int64 {
	m.Lock()
	path := m.dir + "/" + fmt.Sprint(k)
	buf, _ := ioutil.ReadFile(path)
	old, _ := strconv.ParseInt(string(buf), 10, 64)
	ioutil.WriteFile(path, []byte(fmt.Sprint(v)), 0777)
	m.Unlock()
	return old
}

func (m *FSBack) Put(k int64, v int64) (int64, bool) {
	m.Lock()
	defer m.Unlock()
	path := m.dir + "/" + fmt.Sprint(k)
	if _, err := os.Stat(path); err == nil {
		buf, _ := ioutil.ReadFile(path)
		old, _ := strconv.ParseInt(string(buf), 10, 64)
		return old, false
	}
	ioutil.WriteFile(path, []byte(fmt.Sprint(v)), 0777)
	return v, true
}
