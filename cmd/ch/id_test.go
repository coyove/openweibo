package main

import (
	"encoding/binary"
	"log"
	"strconv"
	"testing"
)

func TestID(t *testing.T) {
	m = &Manager{}
	for i := 0; i < 100; i++ {
		x := newID()
		v := binary.BigEndian.Uint64(x)
		log.Println(strconv.FormatUint(v, 2))
	}
}
