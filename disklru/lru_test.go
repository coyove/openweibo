package disklru

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"testing"
	"time"
)

func TestDiskLRU(t *testing.T) {
	flag := true
	m, _ := New("lru_cache", 10, time.Second/2, func(key, path string) error {
		if flag {
			return ioutil.WriteFile(path, []byte(key), 0777)
		}
		return fmt.Errorf("")
	})
	for i := 0; i < 100; i++ {
		f, _ := m.Open(strconv.Itoa(i))
		f.Close()
		time.Sleep(time.Millisecond * 100)
	}
	time.Sleep(time.Second)
	flag = false
	for i := 0; i < 100; i++ {
		f, _ := m.Open(strconv.Itoa(i))
		if f != nil {
			buf, _ := ioutil.ReadAll(f)
			f.Close()
			fmt.Println(i, string(buf))
		}
	}
}
