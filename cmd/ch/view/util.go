package view

import (
	"bytes"
	"net/http"
	"strconv"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
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
	UUID    string
	Content string
	Error   string
	ReplyTo string
}

func makeReplyView(g *gin.Context, reply string) ReplyView {
	r := ReplyView{}
	r.UUID = strconv.FormatInt(time.Now().Unix(), 16)
	r.Content = g.Query("content")
	r.Error = g.Query("error")
	r.ReplyTo = reply
	return r
}

var staticHeader = http.Header{
	"Content-Type": []string{"something"},
}

type fakeResponseCatcher struct {
	bytes.Buffer
}

func (w *fakeResponseCatcher) WriteHeader(code int) {}

func (w *fakeResponseCatcher) Header() http.Header { return staticHeader }
