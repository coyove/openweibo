package types

import (
	"encoding/base64"
	"encoding/binary"
	"math/rand"
	"net"
	"unsafe"
)

func (r *Request) newUserHash(name byte) (UserHash, []byte) {
	tmp := make([]byte, 12+16)
	enc := tmp[12:]
	tmp = tmp[:12]

	ip := r.RemoteIPv4
	copy(tmp[:2], ip)
	ua := r.UserAgent()

	var uaHash uint64
	var uaBytes []byte
	for i := len(ua) - 1; i >= 0 && uaHash <= 0x000f_ffff_ffff_ffff; i-- {
		ch := ua[i]
		if ch >= '1' && ch <= '9' {
			uaHash = uaHash*11 + uint64(ch-'0')
			uaBytes = append(uaBytes, ch)
		} else if ch == '0' {
			if x := uaHash % 11; x != 0 && x != 10 {
				uaHash = uaHash * 11
				uaBytes = append(uaBytes, ch)
			}
		} else {
			if x := uaHash % 11; x != 10 {
				uaHash = uaHash*11 + 10
				uaBytes = append(uaBytes, '.')
			}
		}
	}

	for i := 0; i < len(uaBytes)/2; i++ {
		uaBytes[i], uaBytes[len(uaBytes)-i-1] = uaBytes[len(uaBytes)-i-1], uaBytes[i]
	}

	binary.LittleEndian.PutUint64(tmp[2:], uaHash)

	tmp[9] = byte(rand.Int())
	tmp[10] = byte(rand.Int())
	tmp[11] = name

	Config.Runtime.XTEA.Encrypt(tmp[4:], tmp[4:])
	Config.Runtime.XTEA.Encrypt(tmp[:8], tmp[:8])

	base64.URLEncoding.Encode(enc, tmp)
	return UserHash{
		IP:     ip,
		UA:     string(uaBytes),
		Name:   name,
		base64: enc,
	}, enc
}

type UserHash struct {
	IP     net.IP
	UA     string
	Name   byte
	base64 []byte
}

func (uh UserHash) IsRoot() bool {
	return uh.Name == 'r'
}

func (uh UserHash) IsMod() bool {
	return uh.Name == 'r' || uh.Name == 'm'
}

func (uh UserHash) Display() string {
	if uh.Name == 'r' {
		return "root"
	}
	if uh.Name == 'm' {
		return "mod"
	}
	return string(uh.base64)
}

func ParseUserHash(v []byte) UserHash {
	tmp := make([]byte, 12)
	if len(v) > 16 {
		v = v[:16]
	}
	base64.URLEncoding.Decode(tmp, []byte(v))

	Config.Runtime.XTEA.Decrypt(tmp[:8], tmp[:8])
	Config.Runtime.XTEA.Decrypt(tmp[4:], tmp[4:])

	uaHash := binary.LittleEndian.Uint64(tmp[2:])
	uaHash &= 0x00ff_ffff_ffff_ffff

	ip := net.IP(tmp[:4])
	ip[2], ip[3] = 0, 0
	name := tmp[11]

	tmp = tmp[4:4]
	for i := 0; uaHash > 0; i++ {
		m := byte(uaHash % 11)
		uaHash /= 11
		if m == 10 {
			tmp = append(tmp, '.')
		} else {
			tmp = append(tmp, m+'0')
		}
	}

	return UserHash{ip, *(*string)(unsafe.Pointer(&tmp)), name, v}
}
