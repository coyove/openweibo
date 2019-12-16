package view

import (
	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/mv"
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

func getUser(g *gin.Context) *mv.User {
	u, _ := g.Get("user")
	u2, _ := u.(*mv.User)
	return u2
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
