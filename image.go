package main

import (
	"bufio"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/sirupsen/logrus"
)

func HandleImage(w http.ResponseWriter, r *http.Request) {
	if d := types.Config.Domain; d != "" && !strings.Contains(r.Referer(), d) {
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

	if idx1, idx2 := strings.LastIndexByte(p, '-'), strings.LastIndexByte(p, '.'); idx1 < 0 || idx2 < 0 || idx1+1 > idx2-1 {
		w.WriteHeader(400)
		return
	} else if id, _ := strconv.ParseUint(p[idx1+1:idx2-1], 16, 64); id == 0 {
		w.WriteHeader(400)
		return
	} else if note, _ := dal.GetNote(id); !note.Valid() {
		w.WriteHeader(404)
		return
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

	dal.MetricsIncr("image", float64(n))
}

func saveFile(r *types.Request, id uint64, ts int64, ext string, fileSize int,
	img multipart.File, hdr *multipart.FileHeader) (string, string) {
	fn := fmt.Sprintf("%s%03d(%d)-%x%s", time.Unix(0, ts).Format("060102150405"), (ts/1e6)%1000, fileSize, id, ext)
	path := dal.ImageCacheDir + fn
	os.MkdirAll(filepath.Dir(path), 0777)
	out, err := os.Create(path)
	if err != nil {
		logrus.Errorf("create image %s err: %v", path, err)
		return "", "INTERNAL_ERROR"
	}
	defer out.Close()

	rd := bufio.NewReader(img)
	//buf, _ := rd.Peek(512)
	//switch typ := http.DetectContentType(buf); typ {
	//case "application/pdf", "application/mp4", "video/mp4", "audio/mp4":
	//default:
	//	if strings.HasPrefix(typ, "video/") {
	//	} else if bytes.HasPrefix(buf, []byte{0x6D, 0x6F, 0x6F, 0x76}) { // .mov
	//	} else if !strings.Contains(typ, "image") {
	//		fmt.Println(typ, buf[:10])
	//		return "", "INVALID_IMAGE"
	//	}
	//}

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
		if strings.HasSuffix(x, "a") {
			return ""
		}
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
		x := a[:len(a)-len(ext)]
		if strings.HasSuffix(x, "a") {
			if p == "thumb" {
				return ""
			}
		}
		if strings.HasSuffix(x, "s") {
			if p == "thumb" {
				p = "image"
			}
		}
	}
	path := "/ns:" + p + "/" + a
	if types.Config.ImageDomain != "" {
		return "https://" + types.Config.ImageDomain + path
	}
	return path
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
