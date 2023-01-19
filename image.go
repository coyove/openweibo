package main

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/sirupsen/logrus"
)

func deleteImage(id string) {
	if id == "" {
		return
	}
	os.Remove("image_cache/" + id)
}

func saveImage(r *types.Request, id uint64, seed int, ext string,
	img multipart.File, hdr *multipart.FileHeader) (string, string) {
	fn := fmt.Sprintf("%d/%x%04x-%x%s", types.Uint64Hash(id)%1024, clock.Unix(), seed, id, ext)
	path := "image_cache/" + fn
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
	return fn, ""
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

func parseImageURLSize(in string) (wh [2]int) {
	ext := filepath.Ext(in)
	in = in[:len(in)-len(ext)]
	idx := strings.LastIndexByte(in, '_')
	if idx <= 0 {
		return wh
	}

	var flag byte
	fmt.Sscanf(in[idx+1:], "%dx%d%c", &wh[0], &wh[1], &flag)
	return
}
