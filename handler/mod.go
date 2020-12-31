package handler

import (
	"fmt"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func ModUser(g *gin.Context) {
	var p struct {
		User *model.User
		You  *model.User
		Keys map[string]string
		Raw  string
	}

	p.You = getUser(g)
	if p.You == nil || !p.You.IsMod() {
		NotFound(g)
		return
	}

	p.User, _ = dal.GetUser(g.Query("uid"))
	if p.User == nil {
		NotFound(g)
		return
	}

	if g.Query("swap") == "1" && p.You.IsAdmin() {
		g.SetCookie("id", ik.MakeUserToken(p.User.ID, p.User.Session), 86400, "", "", false, false)
	}

	getter := func(h ik.IDHeader) string {
		id := ik.NewID(h, p.User.ID).String()
		a, _ := dal.GetArticle(id)
		if a == nil {
			return id + " (empty)"
		}
		return id + " → " + a.NextID + " → " + a.EOC + " (EOC)"
	}

	p.Keys = map[string]string{
		"Follower": getter(ik.IDFollower),
		"Follow":   getter(ik.IDFollowing),
		"Block":    getter(ik.IDBlacklist),
		"Like":     getter(ik.IDLike),
		"Timeline": getter(ik.IDAuthor),
		"Inbox":    getter(ik.IDInbox),
	}

	p.Raw = fmt.Sprintf("%+v", p.User)
	g.HTML(200, "mod_user.html", p)
}

func ModKV(g *gin.Context) {
	p := struct {
		You *model.User
		Key string
	}{
		You: getUser(g),
		Key: g.Query("key"),
	}

	if p.You == nil || !p.You.IsAdmin() {
		NotFound(g)
		return
	}

	g.HTML(200, "mod_kv.html", p)
}

func APIBan(g *gin.Context) {
	u := dal.GetUserByContext(g)
	throw(u, "")
	throw(!u.IsAdmin(), "")
	throw(common.Err2(dal.DoUpdateUser(g.PostForm("to"), "Banned", true)), "")
	okok(g)
}

func APIPromoteMod(g *gin.Context) {
	u := dal.GetUserByContext(g)
	throw(u, "")
	throw(!u.IsAdmin(), "")
	throw(common.Err2(dal.DoUpdateUser(g.PostForm("to"), func(u *model.User) error {
		if u.IsAdmin() {
			return fmt.Errorf("e:already_admin")
		}
		if u.Role == "mod" {
			u.Role = ""
		} else {
			u.Role = "mod"
		}
		return nil
	})), "")
	okok(g)
}

func APIModKV(g *gin.Context) {
	u := dal.GetUserByContext(g)
	throw(u, "")
	throw(!u.IsAdmin(), "")

	if g.PostForm("method") == "set" {
		throw(dal.ModKV().Set(g.PostForm("key"), []byte(g.PostForm("value"))), "")
		okok(g)
	} else {
		v, err := dal.ModKV().Get(g.PostForm("key"))
		throw(err, "")
		okok(g, string(v))
	}
}
