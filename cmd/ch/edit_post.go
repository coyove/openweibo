package main

import (
	"log"

	"github.com/gin-gonic/gin"
)

func handleEditPostView(g *gin.Context) {
	var pl = struct {
		UUID    string
		Reply   string
		Tags    []string
		Article *Article
	}{
		UUID:  makeCSRFToken(g),
		Reply: g.Param("id"),
		Tags:  config.Tags,
	}

	a, err := m.GetArticle(displayIDToObejctID(pl.Reply))
	if err != nil {
		log.Println(err)
		g.AbortWithStatus(400)
		return
	}

	pl.Article = a
	g.HTML(200, "editpost.html", pl)
}

func handleEditPostAction(g *gin.Context) {
	if !g.GetBool("ip-ok") || !isCSRFTokenValid(g, g.PostForm("uuid")) {
		g.String(400, "guard/cooling-down")
		return
	}

	var (
		eid       = displayIDToObejctID(g.PostForm("reply"))
		title     = softTrunc(g.PostForm("title"), 100)
		content   = softTrunc(g.PostForm("content"), int(config.MaxContent))
		author    = authorNameToHash(g.PostForm("author"))
		tags      = splitTags(g.PostForm("tags"))
		deleted   = g.PostForm("delete") != ""
		announced = g.PostForm("announce") != ""
		locked    = g.PostForm("locked") != ""
		delimg    = g.PostForm("delimg") != ""
	)

	a, err := m.GetArticle(eid)
	if err != nil {
		g.Redirect(302, "/")
		return
	}

	redir := "/p/" + a.DisplayID()

	if announced && !a.Announcement {
		if isAdmin(author) {
			m.AnnounceArticle(eid)
		}
		g.Redirect(302, redir)
		return
	}

	if locked != a.Locked {
		if isAdmin(author) {
			m.LockArticle(eid, locked)
		}
		g.Redirect(302, redir)
		return
	}

	if a.Author != author && !isAdmin(author) {
		g.Redirect(302, redir)
		return
	}

	if !deleted {
		if a.Parent == "" && len(title) < int(config.MinContent) {
			g.String(400, "title/too-short")
			return
		}
		if len(content) < int(config.MinContent) {
			g.String(400, "content/too-short")
			return
		}
		if a.Locked {
			g.String(400, "guard/post-locked")
			return
		}
	}

	if delimg {
		a.Images = nil
	}

	if err := m.UpdateArticle(a.ID, deleted, title, content, a.Images, tags); err != nil {
		log.Println(err)
		g.String(500, "internal/error")
		return
	}

	if deleted {
		g.Redirect(302, "/")
		return
	}

	if a.Parent != "" {
		g.Redirect(302, "/p/"+a.DisplayParentID())
	} else {
		g.Redirect(302, "/p/"+a.DisplayID())
	}
}
