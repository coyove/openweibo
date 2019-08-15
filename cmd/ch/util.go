package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	_ "image/png"
	"math/rand"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coyove/ch"
	"github.com/coyove/ch/cache"
	"github.com/coyove/common/lru"
	"github.com/gin-gonic/gin"
	"github.com/globalsign/mgo/bson"
)

var (
	mgr       ch.Nodes
	cachemgr  *cache.Cache
	dedup     *lru.Cache
	bytesPool = &sync.Pool{New: func() interface{} { return &bytes.Buffer{} }}
	rxSan     = regexp.MustCompile(`(<|https?://\S+)`)
)

func handleCurrentStat(g *gin.Context) {
	type nodeView struct {
		Name       string
		Capacity   string
		Throt      string
		Free       string
		Error      string
		Offline    bool
		Ping       int64
		LastUpdate string
	}

	p := struct {
		Nodes  []nodeView
		Config template.HTML
	}{
		Config: template.HTML(config.publicString),
	}

	for _, n := range mgr.Nodes() {
		offline, total, used := n.Space()
		stat := n.Stat()
		p.Nodes = append(p.Nodes, nodeView{
			Name:       n.Name,
			Offline:    offline,
			Capacity:   fmt.Sprintf("%.3fG", float64(total)/1024/1024/1024),
			Free:       fmt.Sprintf("%.3fG", float64(total-used)/1024/1024/1024),
			Ping:       stat.Ping,
			Throt:      stat.Throt,
			LastUpdate: time.Since(stat.UpdateTime).String(),
		})
	}

	g.HTML(200, "stat.html", p)
}

func prettyBSON(m bson.M) string {
	buf, _ := json.MarshalIndent(m, "", "  ")
	return string(buf)
}

func softTrunc(a string, n int) string {
	if len(a) <= n {
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

func makeCSRFToken(g *gin.Context) string {
	var p [16]byte
	exp := time.Now().Add(time.Minute * time.Duration(config.TokenTTL)).Unix()
	binary.BigEndian.PutUint32(p[:], uint32(exp))
	if ip := g.ClientIP(); len(ip) >= 6 {
		copy(p[4:10], ip)
	} else {
		copy(p[4:10], "unknow")
	}
	rand.Read(p[10:])
	config.blk.Encrypt(p[:], p[:])
	return hex.EncodeToString(p[:])
}

func isCSRFTokenValid(g *gin.Context, tok string) bool {
	buf, _ := hex.DecodeString(tok)
	if len(buf) != 16 {
		return false
	}
	config.blk.Decrypt(buf, buf)
	exp := binary.BigEndian.Uint32(buf)
	if now := time.Now(); now.After(time.Unix(int64(exp), 0)) ||
		now.Before(time.Unix(int64(exp)-config.TokenTTL*60, 0)) {
		return false
	}

	var ok bool
	if ip := g.ClientIP(); len(ip) >= 6 {
		ok = strings.HasPrefix(ip, string(buf[4:10]))
	} else {
		ok = string(buf[4:10]) == "unknow"
	}

	if ok {
		if _, existed := dedup.Get(tok); existed {
			return false
		}
		dedup.Add(tok, true)
	}
	return ok
}

func makeChallengeToken() string {
	c := make([]byte, 16)
	for i := 0; i < 6; i++ {
		c[i] = byte(rand.Uint64()) % 10
	}
	rand.Read(c[6:])
	config.blk.Encrypt(c, c)
	return hex.EncodeToString(c)
}

func isChallengeTokenValid(tok string, answer string) bool {
	buf, _ := hex.DecodeString(tok)
	if len(buf) != 16 || len(answer) != 6 {
		return false
	}
	config.blk.Decrypt(buf, buf)
	for i := range answer {
		x := byte(answer[i] - '0')
		if buf[i] != x {
			return false
		}
	}
	if _, existed := dedup.Get(tok); existed {
		return false
	}
	dedup.Add(tok, true)
	return true
}

func splitTags(u string) []string {
	urls := []string{}
NEXT:
	for _, u := range regexp.MustCompile(`[\r\n\s\t,]`).Split(u, -1) {
		if u = strings.TrimSpace(u); len(u) < 3 {
			continue
		}
		if u[0] == '#' {
			u = u[1:]
		}
		u = softTrunc(u, 15)
		for _, u2 := range urls {
			if u2 == u {
				continue NEXT
			}
		}
		if urls = append(urls, u); len(urls) >= int(config.MaxTags) {
			break
		}
	}
	return urls
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

func expandText(in string) string {
	t := bytesPool.Get().(*bytes.Buffer)
	for _, r := range in {
		t.WriteRune(r)
		if r > 128 {
			t.WriteRune(' ')
		}
	}
	x := t.String()
	t.Reset()
	bytesPool.Put(t)
	return x
}

func collapseText(in string) string {
	t := bytesPool.Get().(*bytes.Buffer)
	var lastr rune
	for _, r := range in {
		if r == ' ' {
			if lastr > 128 {
				continue
			}
		}
		t.WriteRune(r)
		lastr = r
	}
	x := t.String()
	t.Reset()
	bytesPool.Put(t)
	return x
}

func isAdmin(g interface{}) bool {
	switch g := g.(type) {
	case *gin.Context:
		ck, _ := g.Request.Cookie("id")
		if ck != nil {
			return ck.Value == config.AdminName
		}
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
		return "<a href='" + in + "' target=_blank>" + in + "</a>"
	})
}
