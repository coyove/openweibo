package types

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
)

func Uint64Bytes(v uint64) []byte {
	var p [8]byte
	binary.BigEndian.PutUint64(p[:], uint64(v))
	return p[:]
}

func BytesUint64(p []byte) uint64 {
	if len(p) == 0 {
		return 0
	}
	return binary.BigEndian.Uint64(p)
}

func UUIDStr() string {
	uuid := make([]byte, 48)
	rand.Read(uuid[:16])
	hex.Encode(uuid[16:], uuid[:16])
	return string(uuid[16:])
}

func DedupUint64(v []uint64) []uint64 {
	if len(v) <= 1 {
		return v
	}
	if len(v) == 2 {
		if v[0] == v[1] {
			return v[:1]
		}
		return v
	}
	m := make(map[uint64]bool, len(v))
	for i := len(v) - 1; i >= 0; i-- {
		if m[v[i]] {
			v = append(v[:i], v[i+1:]...)
			continue
		}
		m[v[i]] = true
	}
	return v
}
