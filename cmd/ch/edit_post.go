package main

import (
	"log"
	"net"

	"github.com/gin-gonic/gin"
)

func handleEditPostView(g *gin.Context) {
	var pl = struct {
		UUID    string
		Reply   string
		Tags    []string
		RAuthor string
		IsAdmin bool
		Article *Article
	}{
		Reply: g.Param("id"),
		Tags:  config.Tags,
	}

	pl.UUID, _ = makeCSRFToken(g)
	pl.RAuthor, _ = g.Cookie("id")
	pl.IsAdmin = isAdmin(pl.RAuthor)

	a, err := m.GetArticle(displayIDToObejctID(pl.Reply))
	if err != nil {
		log.Println(err)
		g.Redirect(302, "/vec")
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

	if _, ok := extractCSRFToken(g, g.PostForm("uuid")); !ok {
		errorPage(400, "guard/token-expired", g)
		return
	}

	var (
		eid         = displayIDToObejctID(g.PostForm("reply"))
		title       = softTrunc(g.PostForm("title"), 100)
		content     = softTrunc(g.PostForm("content"), int(config.MaxContent))
		author      = softTrunc(g.PostForm("author"), 32)
		tags        = splitTags(g.PostForm("tags"))
		locked      = g.PostForm("locked") != ""
		highlighted = g.PostForm("highlighted") != ""
	)

	if !isAdmin(author) {
		g.Redirect(302, "/")
		return
	}

	a, err := m.GetArticle(eid)
	if err != nil {
		g.Redirect(302, "/vec")
		return
	}

	redir := "/p/" + a.DisplayID()

	if locked != a.Locked {
		a.Locked = locked
		m.UpdateArticle(a, a.Tags, false)
		g.Redirect(302, redir)
		return
	}

	if highlighted != a.Highlighted {
		a.Highlighted = highlighted
		m.UpdateArticle(a, a.Tags, false)
		g.Redirect(302, redir)
		return
	}

	if a.Parent == 0 && len(title) == 0 {
		errorPage(400, "title/too-short", g)
		return
	}

	if len(content) == 0 {
		errorPage(400, "content/too-short", g)
		return
	}

	oldtags := a.Tags
	a.Content, a.Tags = content, tags

	if a.Parent == 0 {
		a.Title = title
	}

	if err := m.UpdateArticle(a, oldtags, false); err != nil {
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

	if _, ok := extractCSRFToken(g, g.PostForm("uuid")); !ok {
		errorPage(400, "guard/token-expired", g)
		return
	}

	var eid = displayIDToObejctID(g.PostForm("reply"))
	var author = softTrunc(g.PostForm("author"), 32)

	a, err := m.GetArticle(eid)
	if err != nil {
		g.Redirect(302, "/vec")
		return
	}

	if a.Author != authorNameToHash(author) && !isAdmin(author) {
		log.Println(g.MustGet("ip").(net.IP), "tried to delete", a.ID)
		g.Redirect(302, "/p/"+a.DisplayID())
		return
	}

	if err := m.UpdateArticle(a, nil, true); err != nil {
		log.Println(err)
		errorPage(500, "internal/error", g)
		return
	}

	if a.Parent != 0 {
		g.Redirect(302, "/p/"+a.DisplayParentID())
	} else {
		g.Redirect(302, "/vec")
	}
}
