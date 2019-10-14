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

func BytesString(g *gin.Context, p []byte) string {
	key := [4]byte{}
	copy(key[:], g.MustGet("ip").(net.IP))
	x := EncryptArticleID(p, key)
	return strings.Replace(x, prefix, ".", 1)
}

func BytesPlainString(p []byte) string {
	x := idEncoding.EncodeToString(p)
	return strings.Replace(x, prefix, ".", 1)
}

func StringBytes(g *gin.Context, s string) []byte {
	if len(s) < 10 {
		return nil
	}
	s = strings.Replace(s, ".", prefix, 1)
	key := [4]byte{}
	copy(key[:], g.MustGet("ip").(net.IP))
	return DecryptArticleID(s, key)
}
