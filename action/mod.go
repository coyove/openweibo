package action

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/coyove/iis/dal"
	"github.com/gin-gonic/gin"
)

func APIBan(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil || !u.IsMod() {
		g.String(200, "internal/error")
		return
	}

	if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{ID: g.PostForm("to"), ToggleBan: aws.Bool(true)}); err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
}

func APIPromoteMod(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil || !u.IsAdmin() {
		g.String(200, "internal/error")
		return
	}

	if _, err := dal.DoUpdateUser(&dal.UpdateUserRequest{ID: g.PostForm("to"), ToggleMod: aws.Bool(true)}); err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
}

func APIModKV(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil || !u.IsAdmin() {
		g.String(200, "internal/error")
		return
	}

	if g.PostForm("method") == "set" {
		err := dal.ModKV().Set(g.PostForm("key"), []byte(g.PostForm("value")))
		if err != nil {
			g.String(200, err.Error())
		} else {
			g.String(200, "ok")
		}
	} else {
		v, err := dal.ModKV().Get(g.PostForm("key"))
		if err != nil {
			g.String(200, err.Error())
		} else {
			g.String(200, "ok:"+string(v))
		}
	}
}
