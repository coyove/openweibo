package main

import (
	"bytes"
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
	"sync"
	"time"

	"github.com/coyove/ch"
	"github.com/coyove/ch/cache"
	"github.com/coyove/ch/driver"
	"github.com/coyove/common/sched"
)

var (
	mgr      ch.Nodes
	mgrStats sync.Map
	cachemgr *cache.Cache
	client   = &http.Client{
		Timeout: time.Second,
	}
)

func updateStat() {
	for _, n := range mgr.Nodes() {
		mgrStats.Store(n.Name, n.Stat())
		log.Println("[stat] updated:", n.Name)
	}
	sched.Schedule(func() { go updateStat() }, time.Minute)
}

func formatWebURL(u string) string {
	u2, err := url.Parse(u)
	if err != nil {
		return ""
	}
	if u2.Scheme != "https" && u2.Scheme != "http" {
		return ""
	}
	if u2.Host == "" {
		return ""
	}
	return u2.Host + "/" + u2.EscapedPath()
}

func currentStat() interface{} {
	type nodeView struct {
		Name       string
		Capacity   string
		Free       string
		Ping       int64
		LastUpdate string
	}

	p := struct {
		Nodes []nodeView
	}{}

	for _, n := range mgr.Nodes() {
		stati, _ := mgrStats.Load(n.Name)
		stat, _ := stati.(driver.Stat)

		p.Nodes = append(p.Nodes, nodeView{
			Name:       n.Name,
			Capacity:   fmt.Sprintf("%dG", n.Weight),
			Free:       fmt.Sprintf("%.3fM", float64(stat.AvailableBytes)/1024/1024),
			Ping:       stat.Ping,
			LastUpdate: time.Since(stat.UpdateTime).String(),
		})
	}

	return p
}

func extURL(u string) string {
	u2, err := url.Parse(u)
	fmt.Println(u2.Path)
	if err != nil {
		return ""
	}
	return strings.ToLower(filepath.Ext(u2.Path))
}

func fetchImageAsJPEG(url string) ([]byte, image.Point, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, image.ZP, err
	}
	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, image.ZP, err
	}

	img, format, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, image.ZP, err
	}

	if format != "jpeg" {
		x := bytes.Buffer{}
		if err := jpeg.Encode(&x, img, &jpeg.Options{Quality: 80}); err != nil {
			return nil, image.ZP, err
		}
		return x.Bytes(), img.Bounds().Max, nil
	}

	return buf, img.Bounds().Max, nil
}
