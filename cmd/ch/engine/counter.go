package engine

import (
	"encoding/binary"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

var Survey struct {
	Max     int64
	Written int64
}

func loadTrafficCounter() {
	tmppath := filepath.Join(os.TempDir(), "iis")
	traffic, _ := ioutil.ReadFile(tmppath)
	if len(traffic) == 8 {
		Survey.Written = int64(binary.BigEndian.Uint64(traffic[:]))
	}

	go func() {
		last := Survey.Written
		for {
			time.Sleep(time.Minute)

			traffic := [8]byte{}
			binary.BigEndian.PutUint64(traffic[:], uint64(Survey.Written))

			if Survey.Written != last {
				// log.Println("[Traffic recorder] traffic:", Survey.Written, ioutil.WriteFile(tmppath, traffic[:], 0700))
			}

			last = Survey.Written
		}
	}()
}
