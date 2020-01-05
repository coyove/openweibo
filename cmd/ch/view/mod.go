package view

import (
	"fmt"

	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

func ModUser(g *gin.Context) {
	var p struct {
		User *mv.User
		You  *mv.User
		Keys map[string]string
		Raw  string
	}

	p.You = getUser(g)
	if p.You == nil || !p.You.IsMod() {
		NotFound(g)
		return
	}

	p.User, _ = m.GetUser(g.Query("uid"))
	if p.User == nil {
		NotFound(g)
		return
	}

	if g.Query("swap") == "1" && p.You.IsAdmin() {
		g.SetCookie("id", mv.MakeUserToken(p.User), 86400, "", "", false, false)
	}

	getter := func(h ident.IDTag) string {
		id := ident.NewID(h).SetTag(p.User.ID).String()
		a, _ := m.GetArticle(id)
		if a == nil {
			return id + " (empty)"
		}
		return id + " → " + a.NextID + " → " + a.EOC + " (EOC)"
	}

	p.Keys = map[string]string{
		"Follower": getter(ident.IDTagFollowerChain),
		"Follow":   getter(ident.IDTagFollowChain),
		"Block":    getter(ident.IDTagBlockChain),
		"Like":     getter(ident.IDTagLikeChain),
		"Timeline": getter(ident.IDTagAuthor),
		"Inbox":    getter(ident.IDTagInbox),
	}

	p.Raw = fmt.Sprintf("%+v", p.User)
	g.HTML(200, "mod_user.html", p)
}

func ModKV(g *gin.Context) {
	p := struct {
		You *mv.User
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
