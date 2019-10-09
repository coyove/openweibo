package main

import (
	_ "image/png"
	"math"
	"net/url"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/gin-gonic/gin"
)

func checkCategory(u string) string {
	if u == "default" {
		return u
	}
	if !config.Cfg.TagsMap[u] {
		return "default"
	}
	return u
}

func encodeQuery(a ...string) string {
	query := url.Values{}
	for i := 0; i < len(a); i += 2 {
		if a[i] != "" {
			query.Add(a[i], a[i+1])
		}
	}
	return query.Encode()
}

func errorPage(code int, msg string, g *gin.Context) {
	g.HTML(code, "error.html", struct {
		Tags    []string
		Message string
	}{config.Cfg.Tags, msg})
}

func intmin(a, b int) int {
	return int(math.Min(float64(a), float64(b)))
}

func intdivceil(a, b int) int {
	return int(math.Ceil(float64(a) / float64(b)))
}
