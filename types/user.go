package types

import (
	"encoding/base64"
	"encoding/binary"
	"net"

	"github.com/coyove/sdss/contrib/clock"
)

var userHashBase64 = base64.NewEncoding("-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz")

func uaHash(s string) uint64 {
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211
	var hash uint64 = offset64
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == ' ' || (s[i] >= '0' && s[i] <= '9') {
			hash *= prime64
			hash ^= uint64(s[i])
		}
	}
	return uint64(hash)
}

func (r *Request) newUserHash(name byte) (UserHash, []byte) {
	tmp := make([]byte, 6+8)
	enc := tmp[6:]
	tmp = tmp[:6]

	ip := r.RemoteIPv4
	copy(tmp[:3], ip)
	hr := clock.Unix() / 3600
	binary.BigEndian.PutUint16(tmp[3:], uint16(uaHash(r.UserAgent()))*24+uint16(hr%24))
	tmp[5] = name

	Config.Runtime.Skip32.EncryptBytes(tmp[2:])
	Config.Runtime.Skip32.EncryptBytes(tmp[:4])

	userHashBase64.Encode(enc, tmp)
	return UserHash{
		IP:     ip,
		Name:   name,
		base64: enc,
	}, enc
}

type UserHash struct {
	IP     net.IP
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
		return "mod." + string(uh.base64)
	}
	return string(uh.base64)
}

func ParseUserHash(v []byte) UserHash {
	tmp := make([]byte, 6)
	if len(v) >= 8 {
		userHashBase64.Decode(tmp, v[:8])
	}

	Config.Runtime.Skip32.DecryptBytes(tmp[:4])
	Config.Runtime.Skip32.DecryptBytes(tmp[2:])

	ip := net.IP(tmp[:4])
	ip[3] = 0
	name := tmp[5]

	return UserHash{ip, name, v}
}
