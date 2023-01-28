package main

import (
	"bufio"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/sirupsen/logrus"
)

var imageTrafficMap struct {
	hits int64
	atomic.Value
}

func init() {
	imageTrafficMap.Store(new(sync.Map))
}

func sumImageOutboundTraffic() (tot, count int64) {
	imageTrafficMap.Load().(*sync.Map).Range(func(k, v interface{}) bool {
		tot += *v.(*int64)
		count++
		return true
	})
	return
}

func HandleImage(w http.ResponseWriter, r *http.Request) {
	var p string
	if strings.HasPrefix(r.URL.Path, "/ns:image/") {
		p = strings.TrimPrefix(r.URL.Path, "/ns:image/")
	} else {
		p = strings.TrimPrefix(r.URL.Path, "/ns:thumb/")
		ext := filepath.Ext(p)
		p = p[:len(p)-len(ext)] + ".thumb.jpg"
	}
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

	ctr, _ := imageTrafficMap.Load().(*sync.Map).LoadOrStore(p, new(int64)) // basically working
	atomic.AddInt64(ctr.(*int64), int64(n))

	if atomic.AddInt64(&imageTrafficMap.hits, 1)%100 == 0 {
		tot, count := sumImageOutboundTraffic()
		imageTrafficMap.Store(new(sync.Map))
		go func() {
			dal.KVIncr(nil, fmt.Sprintf("daily_image_outbound_traffic_req_%d", clock.Unix()/86400), count)
			dal.KVIncr(nil, fmt.Sprintf("daily_image_outbound_traffic_%d", clock.Unix()/86400), tot)
		}()
	}
}

func deleteImage(id string) {
	if id == "" {
		return
	}
	os.Remove("image_cache/" + id)
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
	if _, err := io.Copy(out, img); err != nil {
		logrus.Errorf("copy image to local %s err: %v", path, err)
		return "", "INTERNAL_ERROR"
	}
	dal.KVIncr(nil, fmt.Sprintf("daily_upload_%v", clock.Unix()/86400), 1)
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
