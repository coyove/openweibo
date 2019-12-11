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
	reply  uint16
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
		hdr:    IDTagGeneral,
		ts:     uint32(time.Now().Unix()),
		tag:    [16]byte{byte(ctr >> 8), byte(ctr), byte(rand.Int()), byte(rand.Int())},
		taglen: 4,
	}
}

func (id ID) Size() int {
	if !id.Valid() {
		return 0
	}
	if id.reply > 0 {
		return int(1 + id.taglen + 4 + 2)
	}
	return int(1 + id.taglen + 4)
}

func (id ID) Marshal(buf []byte) []byte {
	if !id.Valid() {
		return nil
	}
	if len(buf) < id.Size() {
		buf = make([]byte, id.Size())
	}

	if id.reply > 0 {
		buf[0] = byte(id.hdr)<<5 | 0x10 | (id.taglen & 0xf)
		binary.BigEndian.PutUint16(buf[1+id.taglen+4:], id.reply)
	} else {
		buf[0] = byte(id.hdr)<<5 | (id.taglen & 0xf)
	}

	copy(buf[1:], id.tag[:id.taglen])
	binary.BigEndian.PutUint32(buf[1+id.taglen:], id.ts)
	return buf[:id.Size()]
}

func (id ID) Valid() bool { return id.hdr != 0 }

func (id ID) SetTime(t time.Time) ID {
	id.ts = uint32(time.Now().Unix())
	return id
}

func (id ID) Time() time.Time {
	return time.Unix(int64(id.ts), 0)
}

func (id ID) IsRoot() bool {
	return id.ts == 0
}

func (id ID) SetReply(index uint16) ID {
	id.reply = index
	return id
}

func (id ID) Reply() uint16 {
	return id.reply
}

func (id ID) Parent() ID {
	if id.reply == 0 {
		return ID{}
	}
	id.reply = 0
	return id
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
	id.hdr = IDTag(p[0] >> 5)
	id.taglen = p[0] & 0xf
	id.ts = binary.BigEndian.Uint32(p[1+id.taglen:])

	if len(p) < int(1+id.taglen+4) {
		return ID{}
	}

	copy(id.tag[:id.taglen], p[1:1+id.taglen])

	if p[0]>>4&1 == 1 {
		if len(p) < int(1+id.taglen+4+2) {
			return ID{}
		}
		id.reply = binary.BigEndian.Uint16(p[1+id.taglen+4:])
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
