package main

import (
	"crypto/sha1"
	"fmt"
	_ "image/png"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"

	"github.com/coyove/ch/cache"
	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/driver/chdropbox"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v2"
)

func main() {
	buf, err := ioutil.ReadFile("config.yml")
	if err != nil {
		panic(err)
	}

	if err := yaml.Unmarshal(buf, &config); err != nil {
		panic(err)
	}

	log.Println("==== config:", config)

	nodes := []*driver.Node{}
	for _, s := range config.Storages {
		if s.Name == "" {
			panic("empty storage node name")
		}
		log.Println("[config] load storage:", s.Name)
		switch strings.ToLower(s.Type) {
		case "dropbox":
			nodes = append(nodes, chdropbox.NewNode(s.Name, s))
		default:
			panic("unknown storage type: " + s.Type)
		}
	}

	mgr.LoadNodes(nodes)
	mgr.StartTransferAgent("tmp/transfer.db")
	cachemgr = cache.New("tmp/cache", config.CacheSize*1024*1024*1024, mgr.Get)

	r := gin.Default()
	r.LoadHTMLGlob("template/*")
	r.Handle("GET", "/", func(g *gin.Context) {
		g.HTML(200, "index.html", nil)
	})
	r.Handle("GET", "/stat", func(g *gin.Context) {
		g.HTML(200, "stat.html", currentStat())
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
			g.String(500, "[ERR] fetch error: %v", err)
			return
		}
		g.Writer.Header().Add("Content-Type", "image/jpeg") // mime.TypeByExtension(filepath.Ext(url)))
		g.Writer.Write(buf)
	})
	r.Handle("POST", "/crawl", func(g *gin.Context) {
		url := g.PostForm("u")

		if ext := extURL(url); ext != ".jpg" && ext != ".png" {
			g.String(500, "[ERR] invalid url")
			return
		}

		buf, size, err := fetchImageAsJPEG(url)
		if err != nil {
			g.String(500, "[ERR] fetch error: %v", err)
			return
		}

		k := fmt.Sprintf("%x", sha1.Sum([]byte(url)))
		if err := mgr.Put(k, buf); err != nil {
			g.String(500, "[ERR] put error: %v", err)
			return
		}

		if g.PostForm("web") == "1" {
			g.Redirect(302, fmt.Sprintf("/i/%s.jpg", k))
		} else {
			g.String(200, "[OK:%s.jpg] size: %.3fK, dim: %v", k, float64(len(buf))/1024, size)
		}
	})
	log.Fatal(r.Run(":5010"))
}
