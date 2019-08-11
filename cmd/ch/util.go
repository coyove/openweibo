package main

import (
	"bytes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coyove/ch"
	"github.com/coyove/ch/cache"
	"github.com/coyove/ch/driver"
	"github.com/coyove/common/lru"
	"github.com/coyove/common/sched"
	"github.com/gin-gonic/gin"
	"github.com/globalsign/mgo/bson"
)

var (
	mgr       ch.Nodes
	mgrStats  sync.Map
	cachemgr  *cache.Cache
	dedup     *lru.Cache
	bytesPool = &sync.Pool{New: func() interface{} { return &bytes.Buffer{} }}
	client    = &http.Client{
		Timeout: time.Second,
	}
	rxSan      = regexp.MustCompile(`(<|https?://\S+)`)
	rxCRLF     = regexp.MustCompile(`\r?\n`)
	rxNonASCII = regexp.MustCompile(`[^a-zA-Z0-9\-_]`)
	config     = struct {
		Storages []driver.StorageConfig `yaml:"Storages"`
		DynamoDB struct {
			AccessToken string `yaml:"AccessToken"`
			SecretToken string `yaml:"SecretToken"`
			Region      string `yaml:"Region"`
		} `yaml:"DynamoDB"`
		CacheSize    int64    `yaml:"CacheSize"`
		ProdMode     bool     `yaml:"Production"`
		Key          string   `yaml:"Key"`
		TokenTTL     int64    `yaml:"TokenTTL"`
		MaxContent   int64    `yaml:"MaxContent"`
		MinContent   int64    `yaml:"MinContent"`
		MaxTags      int64    `yaml:"MaxTags"`
		AdminName    string   `yaml:"AdminName"`
		PostsPerPage int64    `yaml:"PostsPerPage"`
		Tags         []string `yaml:"Tags"`

		Blk           cipher.Block
		AdminNameHash uint64
	}{
		CacheSize:    1,
		TokenTTL:     1,
		Key:          "0123456789abcdef",
		AdminName:    "zzz",
		MaxContent:   2048,
		MinContent:   10,
		MaxTags:      4,
		PostsPerPage: 30,
		Tags:         []string{"zzz"},
	}
)

func updateStat() {
	for _, n := range mgr.Nodes() {
		mgrStats.Store(n.Name, n.Stat())
		log.Println("[stat] updated:", n.Name)
	}
	sched.Schedule(func() { go updateStat() }, time.Minute)
}

func splitImageURLs(u string) []string {
	urls := []string{}
	for _, u := range regexp.MustCompile(`[\r\n\s\t]`).Split(u, -1) {
		if u = strings.TrimSpace(u); u == "" {
			continue
		}
		u2, err := url.Parse(u)
		if err != nil {
			continue
		} else if u2.Scheme != "https" && u2.Scheme != "http" {
			continue
		} else if u2.Host == "" {
			continue
		} else if len(u2.Path) > 1024 || len(u2.RawPath) > 1024 {
			continue
		}
		urls = append(urls, u2.Host+"/"+u2.EscapedPath())
	}
	return urls
}

func currentStat() interface{} {
	type nodeView struct {
		Name       string
		Capacity   string
		Throt      string
		Free       string
		Error      string
		Ping       int64
		LastUpdate string
	}

	p := struct {
		Nodes []nodeView
	}{}

	for _, n := range mgr.Nodes() {
		stati, _ := mgrStats.Load(n.Name)
		stat, _ := stati.(driver.Stat)

		p.Nodes = append(p.Nodes, nodeView{
			Name:       n.Name,
			Capacity:   fmt.Sprintf("%dG", n.Weight),
			Free:       fmt.Sprintf("%.3fM", float64(stat.AvailableBytes)/1024/1024),
			Ping:       stat.Ping,
			Throt:      stat.Throt,
			LastUpdate: time.Since(stat.UpdateTime).String(),
		})
	}

	return p
}

func extURL(u string) string {
	u2, err := url.Parse(u)
	fmt.Println(u2.Path)
	if err != nil {
		return ""
	}
	return strings.ToLower(filepath.Ext(u2.Path))
}

func fetchImageAsJPEG(url string) ([]byte, image.Point, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, image.ZP, err
	}
	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, image.ZP, err
	}

	img, format, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, image.ZP, err
	}

	if format != "jpeg" {
		x := bytes.Buffer{}
		if err := jpeg.Encode(&x, img, &jpeg.Options{Quality: 80}); err != nil {
			return nil, image.ZP, err
		}
		return x.Bytes(), img.Bounds().Max, nil
	}

	return buf, img.Bounds().Max, nil
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
	config.Blk.Encrypt(p[:], p[:])
	return hex.EncodeToString(p[:])
}

func isCSRFTokenValid(g *gin.Context, tok string) bool {
	buf, _ := hex.DecodeString(tok)
	if len(buf) != 16 {
		return false
	}
	config.Blk.Decrypt(buf, buf)
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
	config.Blk.Encrypt(c, c)
	return hex.EncodeToString(c)
}

func isChallengeTokenValid(tok string, answer string) bool {
	buf, _ := hex.DecodeString(tok)
	if len(buf) != 16 || len(answer) != 6 {
		return false
	}
	config.Blk.Decrypt(buf, buf)
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
