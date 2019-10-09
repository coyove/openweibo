package token

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net"
	"time"

	"github.com/coyove/common/lru"
	"github.com/coyove/iis/cmd/ch/config"
	"github.com/gin-gonic/gin"
)

var Dedup = lru.NewCache(1024)

func Make(g *gin.Context) (string, [6]byte) {
	var p [16]byte
	exp := time.Now().Add(time.Minute * time.Duration(config.Cfg.TokenTTL)).Unix()
	binary.BigEndian.PutUint32(p[:], uint32(exp))
	copy(p[4:10], g.MustGet("ip").(net.IP))
	rand.Read(p[10:])

	var x [6]byte
	copy(x[:], p[10:])

	config.Cfg.Blk.Encrypt(p[:], p[:])
	return hex.EncodeToString(p[:]), x
}

func Parse(g *gin.Context, tok string) (r []byte, ok bool) {
	if IsAdmin(tok) {
		return []byte{0, 0, 0, 0, 0, 0}, true
	}

	buf, _ := hex.DecodeString(tok)
	if len(buf) != 16 {
		return
	}
	config.Cfg.Blk.Decrypt(buf, buf)
	exp := binary.BigEndian.Uint32(buf)
	if now := time.Now(); now.After(time.Unix(int64(exp), 0)) ||
		now.Before(time.Unix(int64(exp)-config.Cfg.TokenTTL*60, 0)) {
		return
	}

	ok = bytes.HasPrefix(buf[4:10], g.MustGet("ip").(net.IP))
	//log.Println(buf[4:10], []byte(g.MustGet("ip").(net.IP)))
	if ok {
		if _, existed := Dedup.Get(tok); existed {
			return nil, false
		}
		Dedup.Add(tok, true)
	}

	r = buf[10:]
	return
}

func IsAdmin(g interface{}) bool {
	switch g := g.(type) {
	case *gin.Context:
		ck, _ := g.Cookie("id")
		if ck != config.Cfg.AdminName {
			return g.PostForm("author") == config.Cfg.AdminName
		}
		return true
	case string:
		return g == config.Cfg.AdminName
	}
	return false
}
