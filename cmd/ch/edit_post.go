package main

import (
	"log"
	"net"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/id"
	mv "github.com/coyove/iis/cmd/ch/modelview"
	"github.com/coyove/iis/cmd/ch/token"
	"github.com/gin-gonic/gin"
)

func handleEditPostView(g *gin.Context) {
	var pl = struct {
		UUID    string
		Reply   string
		Tags    []string
		RAuthor string
		IsAdmin bool
		Article *mv.Article
	}{
		Reply: g.Param("id"),
		Tags:  config.Cfg.Tags,
	}

	pl.UUID, _ = token.Make(g)
	pl.RAuthor, _ = g.Cookie("id")
	pl.IsAdmin = token.IsAdmin(pl.RAuthor)

	a, err := m.Get(id.StringBytes(pl.Reply))
	if err != nil {
		log.Println(err)
		g.Redirect(302, "/cat")
		return
	}
	pl.Article = a

	g.HTML(200, "editpost.html", pl)
}

func handleEditPostAction(g *gin.Context) {
	if !g.GetBool("ip-ok") {
		errorPage(400, "guard/cooling-down", g)
		return
	}

	if _, ok := token.Parse(g, g.PostForm("uuid")); !ok {
		errorPage(400, "guard/token-expired", g)
		return
	}

	var (
		eid         = id.StringBytes(g.PostForm("reply"))
		title       = mv.SoftTrunc(g.PostForm("title"), 100)
		content     = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		author      = getAuthor(g)
		cat         = checkCategory(g.PostForm("cat"))
		locked      = g.PostForm("locked") != ""
		highlighted = g.PostForm("highlighted") != ""
	)

	if !token.IsAdmin(author) {
		g.Redirect(302, "/")
		return
	}

	a, err := m.Get(eid)
	if err != nil {
		g.Redirect(302, "/cat")
		return
	}

	redir := "/p/" + a.DisplayID()

	if locked != a.Locked || highlighted != a.Highlighted {
		a.Locked, a.Highlighted = locked, highlighted
		m.Update(a, a.Category)
		g.Redirect(302, redir)
		return
	}

	if a.Parent == nil && len(title) == 0 {
		errorPage(400, "title/too-short", g)
		return
	}

	if len(content) == 0 {
		errorPage(400, "content/too-short", g)
		return
	}

	oldcat := a.Category
	a.Content, a.Category = content, cat

	if a.Parent == nil {
		a.Title = title
	}

	if err := m.Update(a, oldcat); err != nil {
		log.Println(err)
		errorPage(500, "internal/error", g)
		return
	}

	g.Redirect(302, "/p/"+a.DisplayID())
}

func handleDeletePostAction(g *gin.Context) {
	if !g.GetBool("ip-ok") {
		errorPage(400, "guard/cooling-down", g)
		return
	}

	if _, ok := token.Parse(g, g.PostForm("uuid")); !ok {
		errorPage(400, "guard/token-expired", g)
		return
	}

	var eid = id.StringBytes(g.PostForm("reply"))
	var author = getAuthor(g)

	a, err := m.Get(eid)
	if err != nil {
		g.Redirect(302, "/cat")
		return
	}

	if a.Author != config.HashName(author) && !token.IsAdmin(author) {
		log.Println(g.MustGet("ip").(net.IP), "tried to delete", a.ID)
		g.Redirect(302, "/p/"+a.DisplayID())
		return
	}

	if err := m.Delete(a); err != nil {
		log.Println(err)
		errorPage(500, "internal/error", g)
		return
	}

	if a.Parent != nil {
		g.Redirect(302, "/p/"+a.DisplayParentID())
	} else {
		g.Redirect(302, "/cat")
	}
}
