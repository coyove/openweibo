package view

import (
	"math"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/gin-gonic/gin"
)

func Error(code int, msg string, g *gin.Context) {
	g.HTML(code, "error.html", struct {
		Tags    []string
		Message string
	}{config.Cfg.Tags, msg})
}

func NotFound(g *gin.Context) {
	Error(404, "NOT FOUND", g)
}

func intmin(a, b int) int {
	return int(math.Min(float64(a), float64(b)))
}

func intdivceil(a, b int) int {
	return int(math.Ceil(float64(a) / float64(b)))
}

type ReplyView struct {
	UUID      string
	Content   string
	Error     string
	CanDelete bool
	NSFW      bool
	ReplyTo   string
}

func makeReplyView(g *gin.Context, reply string) ReplyView {
	r := ReplyView{}
	r.UUID, _ = ident.MakeToken(g)
	r.Content = g.Query("content")
	r.Error = g.Query("error")
	r.ReplyTo = reply
	return r
}
