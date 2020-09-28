package ik

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/coyove/iis/common"
)

func fmtdig(sid int64, v int64) byte { return byte((sid/v)%10) + '0' }

func StringifyShortId(id int64) (string, error) {
	sid, err := FormatShortId(id)
	if err != nil {
		return "", err
	}
	buf := []byte{
		fmtdig(sid, 1e11), fmtdig(sid, 1e10), fmtdig(sid, 1e9), fmtdig(sid, 1e8),
		'-',
		fmtdig(sid, 1e7), fmtdig(sid, 1e6), fmtdig(sid, 1e5), fmtdig(sid, 1e4),
		'-',
		fmtdig(sid, 1e3), fmtdig(sid, 1e2), fmtdig(sid, 1e1), fmtdig(sid, 1),
	}
	return *(*string)(unsafe.Pointer(&buf)), nil
}

func FormatShortId(id int64) (int64, error) {
	// Short ID consists 39bits: 4 bits for version, 35 bits for id(int64)

	if id > 0x7ffffffff {
		return 0, fmt.Errorf("too big")
	}

	id = id & 0x7ffffffff

	v := [8]byte{}
	binary.BigEndian.PutUint64(v[:], uint64(id))

	iv := [16]byte{v[7]}
	cipher.NewCTR(common.Cfg.Blk, iv[:]).XORKeyStream(v[4:7], v[4:7])

	tmp := binary.BigEndian.Uint64(v[:])
	tmp |= (1 << 38)                                   // version 1
	tmp = uint64(uint16(tmp>>23)) | (tmp&0x7fffff)<<16 // swap high 16bit and low 23bit
	return int64(tmp) + 1e11, nil                      // +1e11: ensure the decimal rep is 12-digit long
}

func ParseShortId(id int64) (int64, error) {
	id &= 0x7fffffffff
	shortId := uint64(id - 1e11)
	shortId = (shortId >> 16 & 0x7fffff) | uint64(uint16(shortId))<<23

	ver := uint64(shortId) >> 35
	switch ver {
	case 0x8:
		v := [8]byte{}
		binary.BigEndian.PutUint64(v[:], shortId)

		iv := [16]byte{v[7]}
		cipher.NewCTR(common.Cfg.Blk, iv[:]).XORKeyStream(v[4:7], v[4:7])

		{
			tmp := binary.BigEndian.Uint64(v[:])
			tmp = tmp & 0x7ffffffff
			return int64(tmp), nil
		}
	default:
		return 0, fmt.Errorf("invalid short id version: %x", ver)
	}
}
