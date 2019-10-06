package main

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"sync/atomic"
	"time"
)

const (
	IDHdrAnnounce = 0xff
	IDHdrPost     = 0x80
	IDHdrTag      = 0x02
	IDHdrReply    = 0x01
)

type ID struct {
	Hdr    byte
	Tag    string
	RIndex [4]int16
	ts     int64
	ctr    uint32
	rand   uint32
}

var idCounter = rand.Uint32()

func (id *ID) Fill() {
	id.ts = time.Now().Unix()
	id.ctr = atomic.AddUint32(&idCounter, 1) & 0x7ffffff
	id.rand = rand.Uint32() & 0xfffff
}

func (id ID) Marshal() string {
	//  8 + 8x13 + 33 + 27 + 20 + 12x4 = 30
	// hdr  tag    ts  ctr  rand  ridx
	buf := [30]byte{}
	{
		buf[0] = id.Hdr
		copy(buf[1:14], id.Tag)
	}
	{
		tmp := uint64(id.ts)<<31 | uint64(id.ctr)<<4 | uint64(id.rand&0xf)
		binary.BigEndian.PutUint64(buf[14:], tmp)
		binary.BigEndian.PutUint16(buf[22:], uint16(id.rand>>4))
	}
	{
		tmp := [8]byte{}
		binary.BigEndian.PutUint64(tmp[:], uint64(id.RIndex[0])*4096*4096*4096+uint64(id.RIndex[1])*4096*4096+uint64(id.RIndex[2])*4096+uint64(id.RIndex[3]))
		copy(buf[24:], tmp[2:])
	}

	return idEncoding.EncodeToString(buf[:])
}

func (id *ID) Unmarshal(s string) bool {
	p, err := idEncoding.DecodeString(s)
	if err != nil || len(p) != 30 {
		return false
	}

	{
		id.Hdr = p[0]
		end := bytes.IndexByte(p[1:14], 0)
		if end == -1 {
			end = 13
		}
		id.Tag = string(p[1 : end+1])
	}
	{
		tmp := binary.BigEndian.Uint64(p[14:])
		tmp2 := binary.BigEndian.Uint16(p[22:])

		id.ts = int64(tmp >> 31)
		id.ctr = uint32(tmp>>4) & 0x7ffffff
		id.rand = uint32(tmp2)<<4 | uint32(tmp&0xf)
	}
	{
		tmp := binary.BigEndian.Uint64(p[22:]) & 0xffffffffffff
		id.RIndex[0] = int16(tmp>>36) & 0xfff
		id.RIndex[1] = int16(tmp>>24) & 0xfff
		id.RIndex[2] = int16(tmp>>12) & 0xfff
		id.RIndex[3] = int16(tmp) & 0xfff
	}
	return true
}
