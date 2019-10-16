package ident

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	prefix = "0000000000000000"
	zeros  = prefix + prefix
)

func ParseIDString(g *gin.Context, s string) (id ID) {
	if g == nil {
		p, _ := idEncoding.DecodeString(s)
		id.Unmarshal(p)
		return
	}

	id = GDecryptString(g, s)
	return
}

func GEncryptString(g *gin.Context, id ID) string {
	key := [4]byte{}
	copy(key[:], g.MustGet("ip").(net.IP))
	return id.Encrypt(key)
}

func BytesPlainString(p []byte) string {
	x := idEncoding.EncodeToString(p)
	return strings.Replace(x, prefix, ".", 1)
}

func GDecryptString(g *gin.Context, s string) ID {
	key := [4]byte{}
	copy(key[:], g.MustGet("ip").(net.IP))
	id := ID{}
	id.Decrypt(s, key)
	return id
}
