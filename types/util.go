package types

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"
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

func UTF16Trunc(v string, max int) string {
	src, sz := v, 0
	for len(v) > 0 && sz < max {
		r, n := utf8.DecodeRuneInString(v)
		if n == 0 {
			break
		}
		if r > 65535 {
			sz += 2
		} else {
			sz++
		}
		v = v[n:]
	}
	return src[:len(src)-len(v)]
}

func UTF16LenExceeds(v string, max int) bool {
	if len(v) < max {
		return false
	}
	for sz := 0; len(v) > 0; {
		r, n := utf8.DecodeRuneInString(v)
		if n == 0 {
			break
		}
		if r > 65535 {
			sz += 2
		} else {
			sz++
		}
		if sz > max {
			return true
		}
		v = v[n:]
	}
	return false
}

func RemoteIPv4(r *http.Request) net.IP {
	xff := r.Header.Get("X-Forwarded-For")
	ips := strings.Split(xff, ",")
	for _, ip := range ips {
		p := net.ParseIP(strings.TrimSpace(ip))
		if p != nil {
			return p.To4()
		}
		break
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	p := net.ParseIP(ip)
	if p != nil {
		return p.To4()
	}
	return net.IP{0, 0, 0, 0}
}
