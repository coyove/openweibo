package action

import (
	"fmt"

	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func APIBan(g *gin.Context) {
	u := m.GetUserByContext(g)
	if u == nil || !u.IsMod() {
		g.String(200, "internal/error")
		return
	}

	if err := m.UpdateUser(g.PostForm("to"), func(u *model.User) error {
		if u.IsAdmin() {
			return fmt.Errorf("ban/admin-really")
		}
		u.Banned = !u.Banned
		return nil
	}); err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
}

func APIPromoteMod(g *gin.Context) {
	u := m.GetUserByContext(g)
	if u == nil || !u.IsAdmin() {
		g.String(200, "internal/error")
		return
	}

	if err := m.UpdateUser(g.PostForm("to"), func(u *model.User) error {
		if u.IsAdmin() {
			return fmt.Errorf("promote/admin-really")
		}
		if u.Role == "mod" {
			u.Role = ""
		} else {
			u.Role = "mod"
		}
		return nil
	}); err != nil {
		g.String(200, err.Error())
	} else {
		g.String(200, "ok")
	}
}

// func APIResetCache(g *gin.Context) {
// 	u := m.GetUserByContext(g)
// 	if u == nil || !u.IsAdmin() {
// 		g.String(200, "internal/error")
// 		return
// 	}
//
// 	m.ResetCache()
// 	g.String(200, "ok")
// }

func APIModKV(g *gin.Context) {
	u := m.GetUserByContext(g)
	if u == nil || !u.IsAdmin() {
		g.String(200, "internal/error")
		return
	}

	if g.PostForm("method") == "set" {
		err := m.ModKV().Set(g.PostForm("key"), []byte(g.PostForm("value")))
		if err != nil {
			g.String(200, err.Error())
		} else {
			g.String(200, "ok")
		}
	} else {
		v, err := m.ModKV().Get(g.PostForm("key"))
		if err != nil {
			g.String(200, err.Error())
		} else {
			g.String(200, "ok:"+string(v))
		}
	}
}
