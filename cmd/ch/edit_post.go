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
		RAuthor string
		Article *Article
	}{
		UUID:  makeCSRFToken(g),
		Reply: g.Param("id"),
		Tags:  config.Tags,
	}

	pl.RAuthor, _ = g.Cookie("id")

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
		eid        = displayIDToObejctID(g.PostForm("reply"))
		title      = softTrunc(g.PostForm("title"), 100)
		content    = softTrunc(g.PostForm("content"), int(config.MaxContent))
		author     = g.PostForm("author")
		authorHash = authorNameToHash(author)
		tags       = splitTags(g.PostForm("tags"))
		deleted    = g.PostForm("delete") != ""
		announce   = g.PostForm("announce") != ""
		locked     = g.PostForm("locked") != ""
		delimg     = g.PostForm("delimg") != ""
	)

	a, err := m.GetArticle(eid)
	if err != nil {
		g.Redirect(302, "/")
		return
	}

	redir := "/p/" + a.DisplayID()

	if locked != a.Locked {
		if isAdmin(author) {
			a.Locked = true
			m.UpdateArticle(a, a.Tags, false)
		}
		g.Redirect(302, redir)
		return
	}

	if a.Author != authorHash && !isAdmin(author) {
		g.Redirect(302, redir)
		return
	}

	if announce {
		if isAdmin(author) {
			m.UpdateArticle(a, a.Tags, true)
			a := m.NewArticle(a.Title, a.Content, a.Author, a.IP, a.Image, a.Tags)
			a.ID = newBigID()
			a.Announce = true
			m.PostArticle(a)
		}
		g.Redirect(302, "/")
		return
	}

	if !deleted {
		if a.Parent == 0 && len(title) < int(config.MinContent) {
			errorPage(400, "title/too-short", g)
			return
		}
		if len(content) < int(config.MinContent) {
			errorPage(400, "content/too-short", g)
			return
		}
		if a.Locked {
			errorPage(400, "guard/post-locked", g)
			return
		}
	}

	if delimg {
		a.Image = ""
	}

	oldtags := a.Tags
	a.Content, a.Tags = content, tags

	if a.Parent == 0 {
		a.Title = title
	}

	if err := m.UpdateArticle(a, oldtags, deleted); err != nil {
		log.Println(err)
		errorPage(500, "internal/error", g)
		return
	}

	if deleted {
		g.Redirect(302, "/")
		return
	}

	if a.Parent != 0 {
		g.Redirect(302, "/p/"+a.DisplayParentID())
	} else {
		g.Redirect(302, "/p/"+a.DisplayID())
	}
}
