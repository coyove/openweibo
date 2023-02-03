package main

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/sirupsen/logrus"
)

func HandleImage(w http.ResponseWriter, r *http.Request) {
	if d := types.Config.Domain; d != "" && !strings.Contains(r.Referer(), d) {
		time.Sleep(time.Duration(rand.Intn(500)+500) * time.Millisecond)
		w.WriteHeader(400)
		return
	}

	var p string
	if strings.HasPrefix(r.URL.Path, "/ns:image/") {
		p = strings.TrimPrefix(r.URL.Path, "/ns:image/")
	} else {
		p = strings.TrimPrefix(r.URL.Path, "/ns:thumb/")
		ext := filepath.Ext(p)
		p = p[:len(p)-len(ext)] + ".thumb.jpg"
	}
	p = strings.Replace(p, "/", "", -1)

	f, err := dal.Store.ImageCache.Open(p)
	if err != nil {
		if e, ok := err.(dal.S3Error); ok {
			w.WriteHeader(int(e))
		} else {
			logrus.Errorf("image server error %q: %v", p, err)
			w.WriteHeader(500)
		}
		return
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	buf, _ := rd.Peek(512)
	w.Header().Add("Cache-Control", "public, max-age=8640000")
	w.Header().Add("Content-Type", http.DetectContentType(buf))
	n, _ := io.Copy(w, rd)

	dal.MetricsIncr("image", float64(n))
}

func saveImage(r *types.Request, id uint64, ts int64, ext string,
	img multipart.File, hdr *multipart.FileHeader) (string, string) {
	fn := fmt.Sprintf("%s-%x%s", time.Unix(0, ts).Format("060102150405.000"), id, ext)
	path := dal.ImageCacheDir + fn
	os.MkdirAll(filepath.Dir(path), 0777)
	out, err := os.Create(path)
	if err != nil {
		logrus.Errorf("create image %s err: %v", path, err)
		return "", "INTERNAL_ERROR"
	}
	defer out.Close()

	rd := bufio.NewReader(img)
	buf, _ := rd.Peek(512)
	if !strings.Contains(http.DetectContentType(buf), "image") {
		return "", "INVALID_IMAGE"
	}

	n, err := io.Copy(out, rd)
	if err != nil {
		logrus.Errorf("copy image to local %s err: %v", path, err)
		return "", "INTERNAL_ERROR"
	}

	dal.MetricsIncr("upload", float64(n))
	return fn, ""
}

func imageThumbName(a string) string {
	if a == "" {
		return ""
	}
	ext := filepath.Ext(a)
	if ext != "" {
		x := a[:len(a)-len(ext)]
		if strings.HasSuffix(x, "f") {
			return x + ".thumb.jpg"
		}
	}
	return ""
}

func imageURL(p, a string) string {
	if a == "" {
		return ""
	}
	ext := filepath.Ext(a)
	if ext != "" {
		if strings.HasSuffix(a[:len(a)-len(ext)], "s") {
			if p == "thumb" {
				return "/ns:image/" + a
			}
		}
	}
	return "/ns:" + p + "/" + a
}

func checkImageCache(n *types.Note) (found, uploaded int) {
	check := func(id string) {
		if id == "" {
			return
		}
		if _, err := os.Stat(dal.ImageCacheDir + id); err == nil {
			found++
			if err := dal.UploadS3(id); err == nil {
				uploaded++
			}
		}
	}
	check(n.Image)
	check(imageThumbName(n.Image))
	check(n.ReviewImage)
	check(imageThumbName(n.ReviewImage))
	return
}
