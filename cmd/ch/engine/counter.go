package engine

import (
	"encoding/binary"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"
)

var survey struct {
	max     int64
	written int64
}

func loadTrafficCounter() {
	tmppath := filepath.Join(os.TempDir(), "iis")
	traffic, _ := ioutil.ReadFile(tmppath)
	if len(traffic) == 8 {
		survey.written = int64(binary.BigEndian.Uint64(traffic[:]))
	}

	go func() {
		last := survey.written
		for {
			time.Sleep(time.Minute)

			traffic := [8]byte{}
			binary.BigEndian.PutUint64(traffic[:], uint64(survey.written))

			if survey.written != last {
				log.Println("[Traffic recorder] traffic:", survey.written, ioutil.WriteFile(tmppath, traffic[:], 0700))
			}

			last = survey.written
		}
	}()
}
