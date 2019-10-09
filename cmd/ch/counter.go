package main

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
	if len(traffic) != 8 {
		goto SKIP
	}
	survey.written = int64(binary.BigEndian.Uint64(traffic[:]))

SKIP:
	go func() {
		for {
			time.Sleep(time.Minute)
			tmppath := filepath.Join(os.TempDir(), "iis")
			traffic := [8]byte{}
			binary.BigEndian.PutUint64(traffic[:], uint64(survey.written))
			log.Println("[Traffic recorder] traffic:", survey.written, ioutil.WriteFile(tmppath, traffic[:], 0700))
		}
	}()
}
