package ident

import (
	"net"

	"github.com/gin-gonic/gin"
)

func ParseID(s string) (id ID) {
	p, _ := idEncoding.DecodeString(s)
	id.Unmarshal(p)
	return
}

func ParseDynamicID(g *gin.Context, s string) ID {
	key := [4]byte{}
	copy(key[:], g.MustGet("ip").(net.IP))
	id := ID{}
	id.decrypt(s, key)
	return id
}

func (id ID) DynamicString(g *gin.Context) string {
	key := [4]byte{}
	copy(key[:], g.MustGet("ip").(net.IP))
	return id.encrypt(key)
}

func (id ID) String() string {
	if !id.Valid() {
		return ""
	}
	buf := make([]byte, 20)
	id.marshal(buf[5:])
	idEncoding.Encode(buf, buf[5:])
	return string(buf)
}
