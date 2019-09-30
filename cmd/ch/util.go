package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	_ "image/png"
	"math/rand"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/coyove/common/lru"
	ch "github.com/coyove/iis"
	"github.com/coyove/iis/cache"
	"github.com/gin-gonic/gin"
)

var (
	mgr      ch.Nodes
	cachemgr *cache.Cache
	dedup    *lru.Cache
	rxSan    = regexp.MustCompile(`(>.+|<|https?://\S+)`)
)

func softTrunc(a string, n int) string {
	a = strings.TrimSpace(a)
	if len(a) <= n+2 {
		return a
	}
	a = a[:n+2]
	for len(a) > 0 && a[len(a)-1]>>6 == 2 {
		a = a[:len(a)-1]
	}
	if len(a) == 0 {
		return a
	}
	a = a[:len(a)-1]
	return a + "..."
}

func makeCSRFToken(g *gin.Context) (string, [6]byte) {
	var p [16]byte
	exp := time.Now().Add(time.Minute * time.Duration(config.TokenTTL)).Unix()
	binary.BigEndian.PutUint32(p[:], uint32(exp))
	copy(p[4:10], g.MustGet("ip").(net.IP))
	rand.Read(p[10:])

	var x [6]byte
	copy(x[:], p[10:])

	config.blk.Encrypt(p[:], p[:])
	return hex.EncodeToString(p[:]), x
}

func extractCSRFToken(g *gin.Context, tok string) (r []byte, ok bool) {
	if isAdmin(tok) {
		return []byte{0, 0, 0, 0, 0, 0}, true
	}

	buf, _ := hex.DecodeString(tok)
	if len(buf) != 16 {
		return
	}
	config.blk.Decrypt(buf, buf)
	exp := binary.BigEndian.Uint32(buf)
	if now := time.Now(); now.After(time.Unix(int64(exp), 0)) ||
		now.Before(time.Unix(int64(exp)-config.TokenTTL*60, 0)) {
		return
	}

	ok = bytes.HasPrefix(buf[4:10], g.MustGet("ip").(net.IP))
	//log.Println(buf[4:10], []byte(g.MustGet("ip").(net.IP)))
	if ok {
		if _, existed := dedup.Get(tok); existed {
			return nil, false
		}
		dedup.Add(tok, true)
	}

	r = buf[10:]
	return
}

func checkCategory(u string) string {
	if u == "default" {
		return u
	}
	if !config.tagsMap[u] {
		return "default"
	}
	return u
}

func encodeQuery(a ...string) string {
	query := url.Values{}
	for i := 0; i < len(a); i += 2 {
		if a[i] != "" {
			query.Add(a[i], a[i+1])
		}
	}
	return query.Encode()
}

func isAdmin(g interface{}) bool {
	switch g := g.(type) {
	case *gin.Context:
		ck, _ := g.Cookie("id")
		if ck != config.AdminName {
			return g.PostForm("author") == config.AdminName
		}
		return true
	case string:
		return g == config.AdminName
	}
	return false
}

func sanText(in string) string {
	return rxSan.ReplaceAllStringFunc(in, func(in string) string {
		if in == "<" {
			return "&lt;"
		}
		if strings.HasPrefix(in, ">") {
			return "<code>" + strings.TrimSpace(in[1:]) + "</code>"
		}
		return "<a href='" + in + "' target=_blank>" + in + "</a>"
	})
}

func errorPage(code int, msg string, g *gin.Context) {
	g.HTML(code, "error.html", struct{ Message string }{msg})
}

func intmin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
