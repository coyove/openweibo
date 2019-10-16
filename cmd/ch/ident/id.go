package ident

import (
	"encoding/base64"
	"encoding/binary"
	"math/rand"
	"sync/atomic"
	"time"
	"unsafe"
)

const IDLen = 15

type ID struct {
	rIndex [6]byte
	ts     uint32
	ctr    uint16
	rand   uint32
}

var (
	idCounter  = rand.New(rand.NewSource(time.Now().Unix())).Uint32()
	idEncoding = base64.NewEncoding("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz~").WithPadding('-')
)

func NewID() ID {
	id := ID{}
	id.ts = uint32(time.Now().Unix() - 1e9)
	id.ctr = uint16(atomic.AddUint32(&idCounter, 1))
	id.rand = rand.Uint32() & 0xffffff
	return id
}

func NewTagID(tag string) ID {
	id := ID{}
	x := (*[IDLen]byte)(unsafe.Pointer(&id))
	(*x)[0] = 0xff
	copy((*x)[1:], tag)
	return id
}

func (id ID) marshal(buf []byte) []byte {
	if !id.Valid() {
		return nil
	}
	if buf == nil {
		buf = make([]byte, IDLen)
	}
	binary.BigEndian.PutUint32(buf, id.ts)
	binary.BigEndian.PutUint32(buf[5:], id.rand)
	binary.BigEndian.PutUint16(buf[4:], id.ctr)
	copy(buf[9:], id.rIndex[:])
	return buf
}

func (id *ID) Invalidate() {
	id.ts = 0xBADC0DE
}

func (id ID) Valid() bool {
	return id.ts != 0xBADC0DE // timestamp: 0xBADC0DE + 1e9 is approx. equal to 2007/Nov/15
}

func (id *ID) Unmarshal(p []byte) *ID {
	id.Invalidate()

	if len(p) != IDLen {
		return id
	}

	id.ts = binary.BigEndian.Uint32(p)
	id.ctr = binary.BigEndian.Uint16(p[4:])
	id.rand = binary.BigEndian.Uint32(p[5:]) & 0xffffff
	copy(id.rIndex[:], p[9:])

	return id
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

func (id ID) RIndexParent() (parent, topParent ID) {
	pos := make([]int, 0, 6)
	id.RIndexLen(&pos)

	if len(pos) == 0 {
		parent.Invalidate()
		topParent.Invalidate()
		return
	}

	for i := pos[len(pos)-1]; i < len(id.rIndex); i++ {
		id.rIndex[i] = 0
	}

	top := id
	top.rIndex = [6]byte{}

	return id, top
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
