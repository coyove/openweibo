package ident

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"math/rand"
	"sync/atomic"
	"time"
)

type headerType byte

const IDLen = 30

const (
	HeaderInvalid   headerType = 0
	HeaderArticle              = 0x01
	HeaderAuthorTag            = 0x04
	HeaderTimeline             = 0xf0
	HeaderAnnounce             = 0xff
)

type ID struct {
	hdr    headerType
	rIndex [6]byte
	tag    string
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
	if id.hdr == HeaderInvalid {
		return nil
	}
	//  8 + 8x13 + 33 + 27 + 20 + 12x4 = 30
	// hdr  tag    ts  ctr  rand  ridx
	buf := [IDLen]byte{}
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
		copy(buf[24:], id.rIndex[:])
	}

	return buf[:]
}

func (id *ID) Unmarshal(p []byte) bool {
	id.hdr = HeaderInvalid

	if len(p) != IDLen {
		return false
	}
	id.hdr = headerType(p[0])
	end := bytes.IndexByte(p[1:14], 0)
	if end == -1 {
		end = 13
	}
	id.tag = string(p[1 : end+1])
	tmp := binary.BigEndian.Uint64(p[14:])
	tmp2 := binary.BigEndian.Uint16(p[22:])

	id.ts = int64(tmp >> 31)
	id.ctr = uint32(tmp>>4) & 0x7ffffff
	id.rand = uint32(tmp2)<<4 | uint32(tmp&0xf)

	copy(id.rIndex[:], p[24:])

	return id.hdr != HeaderInvalid
}

func (id ID) RIndex() (v int16) {
	for i := 0; i < len(id.rIndex); i++ {
		r := id.rIndex[i]
		if r > 0 && r < 128 {
			v = int16(r)
			continue
		}
		if r >= 128 {
			if i == len(id.rIndex)-1 {
				// shouldn't happen
				return
			}
			v = int16(r&0x7f)<<8 | int16(id.rIndex[i+1])
			i++
			continue
		}
		break
	}
	return
}

func (id ID) RIndexParent() ID {
	pos := make([]int, 0, 6)
	id.RIndexLen(&pos)
	if len(pos) == 0 {
		return ID{}
	}
	for i := pos[len(pos)-1]; i < len(id.rIndex); i++ {
		id.rIndex[i] = 0
	}
	return id
}

func (id ID) RIndexLen(pos *[]int) int {
	var ln int
	for i := 0; i < len(id.rIndex); i++ {
		if pos != nil {
			*pos = append(*pos, i)
		}

		r := id.rIndex[i]
		if r > 0 && r < 128 {
			ln++
			continue
		}
		if r >= 128 {
			if i == len(id.rIndex)-1 {
				// shouldn't happen
				return 0
			}
			ln++
			i++
			continue
		}

		if pos != nil {
			*pos = (*pos)[:len(*pos)-1]
		}
		break
	}
	return ln
}

func (id *ID) RIndexAppend(v int16) bool {
	if v == 0 || v >= 128*128 {
		panic(v)
	}

	for i := 0; i < len(id.rIndex); i++ {
		r := id.rIndex[i]
		if r > 0 && r < 128 {
			continue
		}
		if r >= 128 {
			i++
			continue
		}
		if v > 127 {
			if i == len(id.rIndex)-1 {
				return false
			}
			id.rIndex[i] = byte(v>>8) | 0x80
			id.rIndex[i+1] = byte(v)
		} else {
			id.rIndex[i] = byte(v)
		}
		return true
	}

	return false
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
