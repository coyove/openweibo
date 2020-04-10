package view

import (
	"fmt"
	"strconv"
	"time"

	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func NotFound(g *gin.Context) {
	p := struct{ Accept bool }{g.GetBool("need-accept")}
	g.HTML(404, "error.html", p)
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

func makeCheckpoints(g *gin.Context) []string {
	r := []string{}

	for y, m := time.Now().Year(), int(time.Now().Month()); ; {
		m--
		if m == 0 {
			y, m = y-1, 12
		}

		if y < 2020 {
			break
		}

		r = append(r, fmt.Sprintf("%04d-%02d", y, m))

		if y == 2020 && m == 1 {
			// 2020-01 is the genesis
			break
		}

		if len(r) >= 6 {
			// return 6 checkpoints (months) at most
			break
		}
	}

	return r
}
