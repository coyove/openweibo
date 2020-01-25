package view

import (
	"strconv"
	"time"

	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func NotFound(g *gin.Context) {
	g.HTML(404, "error.html", nil)
}

func getUser(g *gin.Context) *model.User {
	u, _ := g.Get("user")
	u2, _ := u.(*model.User)
	return u2
}

type ReplyView struct {
	UUID    string
	PID     string
	ReplyTo string
}

func makeReplyView(g *gin.Context, reply string) ReplyView {
	r := ReplyView{}
	r.UUID = strconv.FormatInt(time.Now().UnixNano(), 16)
	r.ReplyTo = reply
	r.PID = g.Query("pid")
	return r
}
