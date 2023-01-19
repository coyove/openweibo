package types

import (
	"encoding/base64"
	"encoding/binary"
	"math/rand"
	"net"
	"unsafe"
)

func (r *Request) newUserHash(name byte) (UserHash, []byte) {
	tmp := make([]byte, 9+12)
	enc := tmp[9:]
	tmp = tmp[:9]

	ip := r.RemoteIPv4
	copy(tmp[:2], ip)
	ua := r.UserAgent()

	var uaHash uint32
	var uaBytes []byte
	for i := len(ua) - 1; i >= 0 && uaHash <= 0x0fffffff; i-- {
		ch := ua[i]
		if ch >= '1' && ch <= '7' {
			uaHash = uaHash*9 + uint32(ch-'0')
			uaBytes = append(uaBytes, ch)
		} else if ch == '0' {
			if x := uaHash % 9; x != 0 && x != 8 {
				uaHash = uaHash * 9
				uaBytes = append(uaBytes, ch)
			}
		} else {
			if x := uaHash % 9; x != 8 {
				uaHash = uaHash*9 + 8
				uaBytes = append(uaBytes, '.')
			}
		}
	}

	for i := 0; i < len(uaBytes)/2; i++ {
		uaBytes[i], uaBytes[len(uaBytes)-i-1] = uaBytes[len(uaBytes)-i-1], uaBytes[i]
	}

	binary.LittleEndian.PutUint32(tmp[2:], uaHash)

	tmp[6] = byte(rand.Int())
	tmp[7] = byte(rand.Int())
	tmp[8] = name

	Config.Runtime.XTEA.Encrypt(tmp[:8], tmp[:8])

	base64.URLEncoding.Encode(enc, tmp)
	if enc[11] == 'A' {
		enc = enc[:11]
	}

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
	tmp := make([]byte, 9)
	if len(v) >= 12 {
		base64.URLEncoding.Decode(tmp, v[:12])
	} else if len(v) == 11 {
		base64.URLEncoding.Decode(tmp, append(v, 'A'))
	}

	Config.Runtime.XTEA.Decrypt(tmp[:8], tmp[:8])

	uaHash := binary.LittleEndian.Uint32(tmp[2:])

	ip := net.IP(tmp[:4])
	ip[2], ip[3] = 0, 0
	name := tmp[8]

	tmp = tmp[4:4]
	for i := 0; uaHash > 0; i++ {
		m := byte(uaHash % 9)
		uaHash /= 9
		if m == 8 {
			tmp = append(tmp, '.')
		} else {
			tmp = append(tmp, m+'0')
		}
	}

	return UserHash{ip, *(*string)(unsafe.Pointer(&tmp)), name, v}
}
