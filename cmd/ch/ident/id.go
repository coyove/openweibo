package ident

import (
	"encoding/base64"
	"encoding/binary"
	"math/rand"
	"sync/atomic"
	"time"
)

const (
	IDTagGeneral IDTag = 0x07
	IDTagTag           = 0x06
	IDTagAuthor        = 0x05
	IDTagInbox         = 0x04
)

type IDTag byte

type ID struct {
	hdr    IDTag
	taglen byte
	ts     uint32
	tag    [16]byte
}

var (
	idCounter  = rand.New(rand.NewSource(time.Now().Unix())).Uint32()
	idEncoding = base64.NewEncoding("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz~").WithPadding('-')
)

func NewID(hdr IDTag) ID {
	return ID{hdr: hdr}
}

func NewGeneralID() ID {
	ctr := atomic.AddUint32(&idCounter, 1)
	return ID{
		hdr: IDTagGeneral,
		ts:  uint32(time.Now().Unix()),
		tag: [16]byte{byte(ctr >> 8), byte(ctr), byte(rand.Int()), byte(rand.Int())},
	}
}

func (id ID) Size() int {
	if !id.Valid() {
		return 0
	}
	if id.hdr == IDTagGeneral {
		return 9
	}
	return int(1 + id.taglen)
}

func (id ID) Marshal(buf []byte) []byte {
	if !id.Valid() {
		return nil
	}
	if len(buf) < id.Size() {
		buf = make([]byte, id.Size())
	}

	buf[0] = byte(id.hdr)<<4 | (id.taglen & 0xf)
	if id.hdr == IDTagGeneral {
		binary.BigEndian.PutUint32(buf[1:], id.ts)
		copy(buf[5:], id.tag[:4])
	} else {
		copy(buf[1:], id.tag[:id.taglen])
	}

	return buf[:id.Size()]
}

func (id ID) Valid() bool {
	return id.hdr != 0
}

func (id ID) Time() time.Time {
	return time.Unix(int64(id.ts), 0)
}

func (id ID) IsRoot() bool {
	return id.ts == 0
}

func (id ID) SetTag(tag string) ID {
	buf := CompressString(tag)
	copy(id.tag[:], buf)
	id.taglen = byte(len(buf))
	return id
}

func (id ID) Tag() string {
	return DecompressString(id.tag[:id.taglen])
}

func (id ID) Header() IDTag {
	return id.hdr
}

func UnmarshalID(p []byte) ID {
	if len(p) < 6 {
		return ID{}
	}

	id := ID{}
	id.hdr = IDTag(p[0] >> 4)
	id.taglen = p[0] & 0xf

	if len(p) < id.Size() {
		return ID{}
	}

	if id.hdr == IDTagGeneral {
		copy(id.tag[:4], p[5:])
		id.ts = binary.BigEndian.Uint32(p[1:])
	} else {
		copy(id.tag[:id.taglen], p[1:])
	}
	return id
}

func ParseID(s string) ID {
	p, _ := idEncoding.DecodeString(s)
	return UnmarshalID(p)
}

func (id ID) String() string {
	return idEncoding.EncodeToString(id.Marshal(nil))
}
