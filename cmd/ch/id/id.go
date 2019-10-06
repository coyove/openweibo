package id

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"math/rand"
	"sync/atomic"
	"time"
)

type headerType byte

const (
	HeaderInvalid   headerType = 0
	HeaderReply                = 0x01
	HeaderAuthorTag            = 0x04
	HeaderPost                 = 0x80
	HeaderAnnounce             = 0xff
)

type ID struct {
	hdr    headerType
	tag    string
	rIndex [4]int16
	ts     int64
	ctr    uint32
	rand   uint32
}

var (
	idCounter  = rand.New(rand.NewSource(time.Now().Unix())).Uint32()
	idEncoding = base64.NewEncoding("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz~").WithPadding('-')
)

func NewID(hdr headerType, tag string) ID {
	id := ID{}
	id.hdr = hdr
	id.tag = tag
	id.ts = time.Now().Unix()
	id.ctr = atomic.AddUint32(&idCounter, 1) & 0x7ffffff
	id.rand = rand.Uint32() & 0xfffff
	return id
}

func ParseID(s interface{}) ID {
	var id ID
	switch s := s.(type) {
	case string:
		id.Unmarshal(StringBytes(s))
	case []byte:
		id.Unmarshal(s)
	}
	return id
}

func (id ID) Header() headerType {
	return id.hdr
}

func (id ID) Tag() string {
	return id.tag
}

func (id ID) String() string {
	return BytesString(id.Marshal())
}

func (id ID) Marshal() []byte {
	//  8 + 8x13 + 33 + 27 + 20 + 12x4 = 30
	// hdr  tag    ts  ctr  rand  ridx
	buf := [30]byte{}
	{
		buf[0] = byte(id.hdr)
		copy(buf[1:14], id.tag)
	}
	{
		tmp := uint64(id.ts&0x1ffffffff)<<31 | uint64(id.ctr&0x7ffffff)<<4 | uint64(id.rand&0xf)
		binary.BigEndian.PutUint64(buf[14:], tmp)
		binary.BigEndian.PutUint16(buf[22:], uint16(id.rand>>4))
	}
	{
		tmp := [8]byte{}
		binary.BigEndian.PutUint64(tmp[:], uint64(id.rIndex[0])*4096*4096*4096+uint64(id.rIndex[1])*4096*4096+uint64(id.rIndex[2])*4096+uint64(id.rIndex[3]))
		copy(buf[24:], tmp[2:])
	}

	return buf[:]
}

func (id *ID) Unmarshal(p []byte) bool {
	if len(p) != 30 {
		return false
	}

	{
		id.hdr = headerType(p[0])
		end := bytes.IndexByte(p[1:14], 0)
		if end == -1 {
			end = 13
		}
		id.tag = string(p[1 : end+1])
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
		id.rIndex[0] = int16(tmp>>36) & 0xfff
		id.rIndex[1] = int16(tmp>>24) & 0xfff
		id.rIndex[2] = int16(tmp>>12) & 0xfff
		id.rIndex[3] = int16(tmp) & 0xfff
	}
	return true
}

func (id ID) RIndex() int16 {
	i := id.RIndexLen()
	if i == 0 {
		return 0
	}
	return id.rIndex[i-1]
}

func (id ID) RIndexLen() int {
	for i, r := range id.rIndex {
		if r == 0 {
			return i
		}
	}
	return 4
}

func (id *ID) RIndexAppend(i int16) {
	x := id.RIndexLen()
	if x == 4 {
		panic("check")
	}
	id.rIndex[x] = i & 0xfff
}

func (id *ID) SetHeader(h headerType) {
	id.hdr = h
}

func (id *ID) SetTag(t string) {
	id.tag = t
}

func (id *ID) Maximize() {
	id.ctr = 0xffffffff
	id.rand = 0xffffffff
	id.ts = 0x1ffffffff
}
