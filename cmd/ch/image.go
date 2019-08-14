package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coyove/ch"
	"github.com/gin-gonic/gin"
)

func isValidImage(v []byte) bool {
	ct := http.DetectContentType(v)
	return strings.HasPrefix(ct, "image/")
}

func getImageLocalTmpPath(filename string, v []byte) (localPath string, displayPath string) {
	key := mgr.MakeKey(v)
	displayPath = key.String() + filepath.Ext(filename)
	localPath = fmt.Sprintf("tmp/images/%d", key[0])
	os.MkdirAll(localPath, 0700)
	localPath += "/" + key.String()
	return
}

func extractImageKey(key string) (ch.Key, string, error) {
	k := ch.Key{}
	key = filepath.Base(key)
	if err := k.FromString(key); err != nil {
		return k, "", err
	}
	return k, fmt.Sprintf("tmp/images/%d/%s", k[0], k.String()), nil
}

func handleImage(g *gin.Context) {
	x := g.Param("image")
	k, localpath, err := extractImageKey(x)
	if err != nil {
		log.Println("[image]", err)
		g.AbortWithStatus(400)
		return
	}
	if _, err := os.Stat(localpath); err == nil {
		// the image exists in the tmp/images folder
		g.File(localpath)
		return
	}

	kstr := k.String()
	cachepath := cachemgr.MakePath(kstr)
	if _, err := os.Stat(cachepath); err == nil {
		g.File(cachepath)
		return
	}

	v, err := cachemgr.Fetch(kstr)
	if err != nil {
		log.Println("[image.fetch]", err)
		g.AbortWithStatus(500)
		return
	}
	g.Writer.Write(v)
}

func uploadLocalImages() {
	for {
		filepath.Walk("tmp/images", func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			log.Println("[upload]", path)

			k := ch.Key{}
			if k.FromString(filepath.Base(path)) != nil {
				os.Remove(path)
				return nil
			}

			v, err := ioutil.ReadFile(path)
			if err != nil {
				return nil
			}

			if k2, err := mgr.Put(v); err != nil {
				log.Println("[upload] upload error:", err)
				return nil
			} else if k2 != k.String() {
				log.Println("[upload] missed keys:", k2, k)
				os.Remove(path)
				return nil
			}

			log.Println("[upload] finished:", path, len(v))
			os.Remove(path)

			cachepath := cachemgr.MakePath(k.String())
			ioutil.WriteFile(cachepath, v, 0700)

			time.Sleep(time.Second)
			return nil
		})
		time.Sleep(time.Second * 10)
	}
}
