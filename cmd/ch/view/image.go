package view

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/gin-gonic/gin"
)

func Home(g *gin.Context) {
	id, _ := g.Cookie("id")

	var p = struct {
		ID        string
		IsCrawler bool
		IsAdmin   bool
		Config    template.HTML
		Tags      []string
	}{
		id,
		manager.IsCrawler(g),
		ident.IsAdmin(g),
		template.HTML(config.Cfg.PublicString),
		config.Cfg.Tags,
	}

	if ident.IsAdmin(g) {
		p.Config = template.HTML(config.Cfg.PrivateString)
	}

	g.HTML(200, "home.html", p)
}

var imgClient = &http.Client{Timeout: 1 * time.Second}

func Image(g *gin.Context) {

	if config.Cfg.Key == "0123456789abcdef" {
		// debug
		imgClient.Transport = &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return url.Parse("socks5://127.0.0.1:1080")
			},
		}
		imgClient.Timeout = 0
	}

	img, _ := base64.StdEncoding.DecodeString(strings.TrimRight(g.Param("img"), ".jpg"))

	hash := sha1.Sum(img)
	cachepath := fmt.Sprintf("tmp/images/%x/%x", hash[0], hash[1:])

	m.Lock(cachepath)
	defer m.Unlock(cachepath)

	if _, err := os.Stat(cachepath); err == nil {
		http.ServeFile(g.Writer, g.Request, cachepath)
		return
	}

	u, err := url.Parse(string(img))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		g.Status(404)
		return
	}

	resp, err := imgClient.Get(u.String())
	if err != nil {
		log.Println("Image Proxy", err)
		g.Status(500)
		return
	}

	defer resp.Body.Close()

	cachedir := filepath.Dir(cachepath)
	os.MkdirAll(cachedir, 0777)

	f, err := os.Create(cachepath)
	if err != nil {
		log.Println("Image Proxy, disk error:", err)
		return
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		log.Println("Image Proxy, disk copy error:", err)
		f.Close()
		os.Remove(cachepath)
		g.Status(500)
	} else {
		f.Close()
		http.ServeFile(g.Writer, g.Request, cachepath)
	}
}

func init() {
	go func() {
		for {
			dirs, _ := ioutil.ReadDir("tmp/images")
			if len(dirs) == 0 {
				time.Sleep(time.Hour)
				continue
			}

			for _, dir := range dirs {
				if !dir.IsDir() {
					continue
				}

				path := filepath.Join("tmp/images", dir.Name())
				files, _ := ioutil.ReadDir(path)

				for _, f := range files {
					if time.Since(f.ModTime()).Seconds() > 86400*3 {
						path := filepath.Join(path, f.Name())
						os.Remove(path)
					}
				}

				time.Sleep(time.Minute)
			}
		}
	}()
}
