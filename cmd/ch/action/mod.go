package action

import (
	"fmt"

	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

func APIBan(g *gin.Context) {
	u := m.GetUserByContext(g)
	if u == nil || !u.IsMod() {
		g.String(200, "internal/error")
		return
	}

	if err := m.UpdateUser(g.PostForm("to"), func(u *mv.User) error {
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

	if err := m.UpdateUser(g.PostForm("to"), func(u *mv.User) error {
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

func APIResetCache(g *gin.Context) {
	u := m.GetUserByContext(g)
	if u == nil || !u.IsAdmin() {
		g.String(200, "internal/error")
		return
	}

	m.ResetCache()
	g.String(200, "ok")
}
