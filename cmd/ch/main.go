package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/coyove/ch"
	"github.com/coyove/ch/cache"
	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/driver/chdropbox"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v2"
)

var mgr ch.Nodes
var cachemgr *cache.Cache

func extURL(u string) string {
	u2, err := url.Parse(u)
	fmt.Println(u2.Path)
	if err != nil {
		return ""
	}
	return strings.ToLower(filepath.Ext(u2.Path))
}

func main() {
	buf, err := ioutil.ReadFile("config.yml")
	if err != nil {
		panic(err)
	}

	config := map[interface{}]interface{}{}
	if err := yaml.Unmarshal(buf, config); err != nil {
		panic(err)
	}

	nodes := []*driver.Node{}
	storages, _ := config["Storages"].([]interface{})
	for _, s := range storages {
		s := s.(map[interface{}]interface{})
		name, ty := driver.Itos(s["Name"], ""), driver.Itos(s["Type"], "")
		if name == "" {
			panic("empty storage node name")
		}
		switch strings.ToLower(ty) {
		case "dropbox":
			log.Println("[config] load storage:", name)
			nodes = append(nodes, chdropbox.NewNode(name, s))
		default:
			panic("unknown storage type: " + ty)
		}
	}

	mgr.LoadNodes(nodes)
	mgr.StartTransferAgent("transfer.db")
	cachemgr = cache.New("cache2", driver.Itoi(config["CacheSize"], 1)*1024*1024*1024,
		func(k string) ([]byte, error) {
			return mgr.Get(k)
		})

	r := gin.Default()
	r.LoadHTMLGlob("template/*")
	r.Handle("GET", "/", func(g *gin.Context) {
		g.HTML(200, "index.html", nil)
	})
	r.Handle("GET", "/i/:url", func(g *gin.Context) {
		url, k := g.Param("url"), ""
		if url == "raw" {
			url = g.Query("u")
			k = fmt.Sprintf("%x", sha1.Sum([]byte(url)))
		} else {
			k = strings.TrimRight(url, filepath.Ext(url))
		}
		buf, err := cachemgr.Fetch(k)
		if err != nil {
			g.String(500, "can't get: %s, error: %v", url, err)
			return
		}
		g.Writer.Header().Add("Content-Type", "image/jpeg") // mime.TypeByExtension(filepath.Ext(url)))
		g.Writer.Write(buf)
	})
	r.Handle("POST", "/crawl", func(g *gin.Context) {
		url := g.PostForm("u")
		ext := extURL(url)
		if ext != ".jpg" && ext != ".png" {
			g.String(500, "[ERR] invalid URL")
			return
		}
		client := &http.Client{Timeout: time.Second}
		resp, err := client.Get(url)
		if err != nil {
			g.String(500, "[ERR] can't fetch: %s, error: %v", url, err)
			return
		}
		defer resp.Body.Close()
		buf, err := ioutil.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		if err != nil || len(buf) == 0 {
			g.String(500, "[ERR] can't fetch: %s, read error: %v", url, err)
			return
		}
		img, _, err := image.Decode(bytes.NewReader(buf))
		if err != nil {
			g.String(500, "[ERR] can't decode: %s, parsing error: %v", url, err)
			return
		}
		if ext == ".png" {
			x := bytes.Buffer{}
			if err := jpeg.Encode(&x, img, &jpeg.Options{Quality: 80}); err != nil {
				g.String(500, "[ERR] can't encode: %s, error: %v", url, err)
				return
			}
			buf, ext = x.Bytes(), ".jpg"
		}
		k := fmt.Sprintf("%x", sha1.Sum([]byte(url)))
		if err := mgr.Put(k, buf); err != nil {
			g.String(500, "[ERR] can't put: %s, error: %v", url, err)
			return
		}
		if g.PostForm("web") == "1" {
			g.Redirect(302, fmt.Sprintf("/i/%s%s", k, ext))
		} else {
			g.String(200, "[OK:%s%s] size: %.3fK, dim: %v", k, ext, float64(len(buf))/1024, img.Bounds().Max)
		}
	})
	r.Run(":5010")
}
