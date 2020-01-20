package action

import (
	"github.com/coyove/iis/dal"
	"github.com/gin-gonic/gin"
)

func APIBan(g *gin.Context) {
	u := dal.GetUserByContext(g)
	if u == nil || !u.IsMod() {
		g.String(200, "internal/error")
		return
	}

	if err := dal.Do(dal.NewRequest(dal.DoUpdateUser,
		"ID", g.PostForm("to"),
		"ToggleBan", true,
	)); err != nil {
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

	if err := dal.Do(dal.NewRequest(dal.DoUpdateUser,
		"ID", g.PostForm("to"),
		"ToggleMod", true,
	)); err != nil {
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
