package ident

import (
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"net"
	"net/url"
	"time"

	"github.com/coyove/common/lru"
	"github.com/coyove/iis/cmd/ch/config"
	"github.com/gin-gonic/gin"
)

var Dedup = lru.NewCache(1024)

func MakeToken(g *gin.Context) (string, [4]byte) {
	var p [16]byte
	exp := time.Now().Add(time.Minute * time.Duration(config.Cfg.TokenTTL)).Unix()
	binary.BigEndian.PutUint32(p[:], uint32(exp))
	copy(p[4:10], g.MustGet("ip").(net.IP))
	rand.Read(p[10:])

	var x [4]byte
	copy(x[:], p[10:])

	config.Cfg.Blk.Encrypt(p[:], p[:])
	return hex.EncodeToString(p[:]), x
}

func ParseToken(g *gin.Context, tok string) (r []byte, ok bool) {
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
		if g == nil {
			return false
		}
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

func MakeTempToken(id string) string {
	if len(id) == 0 {
		return ""
	}

	var nonce [12]byte
	exp := time.Now().Add(time.Second * time.Duration(config.Cfg.IDTokenTTL)).Unix()
	binary.BigEndian.PutUint32(nonce[:], uint32(exp))
	rand.Read(nonce[4:])

	idbuf := make([]byte, len(id), len(id)+48)
	copy(idbuf, id)

	gcm, _ := cipher.NewGCM(config.Cfg.Blk)
	data := gcm.Seal(idbuf[:0], nonce[:], idbuf, nil)
	return base64.URLEncoding.EncodeToString(append(data, nonce[:]...))
}

func ParseTempToken(tok string) string {
	idbuf, _ := base64.URLEncoding.DecodeString(tok)
	if len(idbuf) < 12 {
		return ""
	}

	nonce := idbuf[len(idbuf)-12:]
	idbuf = idbuf[:len(idbuf)-12]

	exp := time.Unix(int64(binary.BigEndian.Uint32(nonce)), 0)
	if time.Now().After(exp) {
		return ""
	}

	gcm, _ := cipher.NewGCM(config.Cfg.Blk)
	p, _ := gcm.Open(nil, nonce, idbuf, nil)
	return string(p)
}

func (id ID) Encrypt(key [4]byte) string {
	if !id.Valid() {
		return ""
	}

	dst := make([]byte, 8+15+4+2+3) // 8+24 bytes, 8 used as base64 buffer

	exp := uint32(time.Now().Add(time.Second *
		time.Duration(config.Cfg.IDTokenTTL)).Unix())

	buf := dst[8:]                            // last 24 bytes
	id.Marshal(buf[:15])                      // inited with 15 bytes id
	binary.BigEndian.PutUint32(buf[15:], exp) // followed by 4 bytes ts
	copy(buf[19:], key[:2])                   // followed by 2 bytes key
	rand.Read(buf[21:])                       // ended with 3 bytes randomness

	config.Cfg.Blk.Encrypt(buf[8:], buf[8:])
	config.Cfg.Blk.Encrypt(buf[:16], buf[:16])

	idEncoding.Encode(dst[:32], buf)
	return string(dst[:32])
}

var decryptArticleIDCheckExp = true

func SetDecryptArticleIDCheckExp(f bool) { decryptArticleIDCheckExp = f }

func (id *ID) Decrypt(tok string, key [4]byte) *ID {
	id.Invalidate()

	buf, _ := idEncoding.DecodeString(tok)
	if len(buf) != 24 {
		if len(buf) == IDLen {
			id.Unmarshal(buf)
		}
		return id
	}

	config.Cfg.Blk.Decrypt(buf[:16], buf[:16])
	config.Cfg.Blk.Decrypt(buf[8:], buf[8:])

	if decryptArticleIDCheckExp {

		exp := time.Unix(int64(binary.BigEndian.Uint32(buf[15:])), 0)
		if time.Now().After(exp) {
			return id
		}

		if buf[19] != key[0] || buf[20] != key[1] {
			return id
		}
	}

	id.Unmarshal(buf[:15])
	return id
}

func DecryptQuery(g *gin.Context) {
	kv := ParseTempToken(g.Query("enc"))
	if kv == "" {
		return
	}
	if g.Request.URL.RawQuery != "" {
		kv += "&" + g.Request.URL.RawQuery
	}
	g.Request.URL.RawQuery = kv
}

func EncryptQuery(a ...string) string {
	query := url.Values{}
	for i := 0; i < len(a); i += 2 {
		if a[i] != "" {
			query.Add(a[i], a[i+1])
		}
	}
	return "?enc=" + url.PathEscape(MakeTempToken(query.Encode()))
}
