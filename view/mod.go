package view

import (
	"fmt"

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

	p.User, _ = m.GetUserWithSettings(g.Query("uid"))
	if p.User == nil {
		NotFound(g)
		return
	}

	if g.Query("swap") == "1" && p.You.IsAdmin() {
		g.SetCookie("id", ik.MakeUserToken(p.User), 86400, "", "", false, false)
	}

	getter := func(h ik.IDTag) string {
		id := ik.NewID(h).SetTag(p.User.ID).String()
		a, _ := m.GetArticle(id)
		if a == nil {
			return id + " (empty)"
		}
		return id + " → " + a.NextID + " → " + a.EOC + " (EOC)"
	}

	p.Keys = map[string]string{
		"Follower": getter(ik.IDTagFollowerChain),
		"Follow":   getter(ik.IDTagFollowChain),
		"Block":    getter(ik.IDTagBlockChain),
		"Like":     getter(ik.IDTagLikeChain),
		"Timeline": getter(ik.IDTagAuthor),
		"Inbox":    getter(ik.IDTagInbox),
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
