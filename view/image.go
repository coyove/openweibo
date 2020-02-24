package view

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/common"
	"github.com/gin-gonic/gin"
)

func Home(g *gin.Context) {
	if getUser(g) != nil {
		g.Redirect(302, "/t")
	} else {
		g.HTML(200, "home.html", nil)
	}
}

var imgClient = &http.Client{Timeout: 1 * time.Second}

func Image(g *gin.Context) {
	img, _ := base64.StdEncoding.DecodeString(strings.TrimRight(g.Param("img"), ".jpg"))

	hash := sha1.Sum(img)
	cachepath := fmt.Sprintf("tmp/images/%x/%x", hash[0], hash[1:])

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

func I(g *gin.Context) {
	img := strings.TrimSuffix(g.Param("img"), ".jpg")
	if len(img) > 16 {
		x, _ := strconv.ParseUint(img[:16], 16, 64)
		cachepath := fmt.Sprintf("tmp/images/%d/%s", x%1024, img)

		if g.Query("thumb") != "" {
			path := cachepath + "@thumb"
			if _, err := os.Stat(path); err != nil {
				path = cachepath
			}
			http.ServeFile(g.Writer, g.Request, path)
		} else {
			http.ServeFile(g.Writer, g.Request, cachepath)
		}
	} else {
		cachepath := fmt.Sprintf("tmp/images/%s/%s", img[:2], img)
		http.ServeFile(g.Writer, g.Request, cachepath)
	}
}

func init() {
	dirMaxSize := common.Cfg.MaxImagesCache * 1024 * 1024 * 1024 / (1024)

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

				totalSize := 0
				for _, f := range files {
					totalSize += int(f.Size())
				}

				if totalSize > dirMaxSize {
					// too many files, purging the oldest
					start, old := time.Now(), totalSize

					sort.Slice(files, func(i, j int) bool {
						return files[i].ModTime().Before(files[j].ModTime())
					})

					for _, f := range files {
						if strings.Contains(f.Name(), "@") {
							// user avatars, keep them whenever possible
							continue
						}

						path := filepath.Join(path, f.Name())
						totalSize -= int(f.Size())
						os.Remove(path)

						if totalSize <= dirMaxSize {
							break
						}
					}

					log.Println("Image Purger:", path, "old size:", old, dirMaxSize, "purged in", time.Since(start))
				} else {
					log.Println("Image Purger OK:", path, "size:", totalSize, dirMaxSize, ":", totalSize*100/dirMaxSize, "%")
				}

				time.Sleep(time.Minute)
			}
		}
	}()
}
