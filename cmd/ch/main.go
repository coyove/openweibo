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
		log.Println("[config] load storage:", name)
		switch strings.ToLower(ty) {
		case "dropbox":
			nodes = append(nodes, chdropbox.NewNode(name, s))
		default:
			panic("unknown storage type: " + ty)
		}
	}

	mgr.LoadNodes(nodes)
	mgr.StartTransferAgent("tmp")
	cachemgr = cache.New("tmp/cache", driver.Itoi(config["CacheSize"], 1)*1024*1024*1024,
		func(k string) ([]byte, error) {
			return mgr.Get(k)
		})

	updateStat()

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
