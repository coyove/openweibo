package types

import (
	"encoding/base64"
	"encoding/binary"
	"math/rand"
	"net"
	"unsafe"
)

func (r *Request) newUserHash(name string) (UserHash, []byte) {
	tmp := make([]byte, 15+20)
	enc := tmp[15:]
	tmp = tmp[:15]

	ip := r.RemoteIPv4()
	copy(tmp[:3], ip)
	ua := r.UserAgent()

	var uaHash uint64
	var uaBytes []byte
	for i := 0; i < len(ua) && uaHash <= 0x00ffffffffffffff; i++ {
		if ua[i] >= '0' && ua[i] <= '9' {
			uaHash = uaHash*36 + uint64(ua[i]-'0'+26)
			uaBytes = append(uaBytes, ua[i])
		}
		if ua[i] >= 'A' && ua[i] <= 'Z' {
			uaHash = uaHash*36 + uint64(ua[i]-'A')
			uaBytes = append(uaBytes, ua[i])
		}
	}

	binary.BigEndian.PutUint64(tmp[3:], uaHash)

	if name == "" {
		for i := 11; i < len(tmp); i++ {
			tmp[i] = byte(rand.Int())
		}
	} else {
		copy(tmp[11:], name)
	}

	base64.URLEncoding.Encode(enc, tmp)
	return UserHash{
		IP:     ip,
		UA:     string(uaBytes),
		Name:   string(tmp[11:]),
		base64: enc,
	}, enc
}

type UserHash struct {
	IP     net.IP
	UA     string
	Name   string
	base64 []byte
}

func (uh UserHash) IsRoot() bool {
	return uh.Name == "root"
}

func (uh UserHash) IsMod() bool {
	return uh.Name == "root" || uh.Name == "mods"
}

func (uh UserHash) Display() string {
	if uh.Name == "root" {
		return uh.Name
	}
	if uh.Name == "mods" {
		return uh.Name
	}
	return string(uh.base64)
}

func ParseUserHash(v []byte) UserHash {
	tmp := make([]byte, 15)
	base64.URLEncoding.Decode(tmp, []byte(v))

	uaHash := binary.BigEndian.Uint64(tmp[3:])

	ip := net.IP(tmp[:4])
	ip[3] = 0
	name := string(tmp[11:])

	tmp = tmp[4:4]
	for i := 0; uaHash > 0; i++ {
		m := byte(uaHash % 36)
		if m < 26 {
			m += 'A'
		} else {
			m = m - 26 + '0'
		}
		uaHash /= 36
		tmp = append(tmp, m)
	}
	for i := 0; i < len(tmp)/2; i++ {
		tmp[i], tmp[len(tmp)-i-1] = tmp[len(tmp)-i-1], tmp[i]
	}
	return UserHash{ip, *(*string)(unsafe.Pointer(&tmp)), name, v}
}
