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
		g.Redirect(302, "/")
		return
	}

	pl.Article = a
	g.HTML(200, "editpost.html", pl)
}

func handleEditPostAction(g *gin.Context) {
	if !g.GetBool("ip-ok") {
		g.String(400, "guard/cooling-down")
		return
	}

	if _, ok := extractCSRFToken(g, g.PostForm("uuid"), true); !ok {
		g.String(400, "guard/token-expired")
		return
	}

	var (
		eid        = displayIDToObejctID(g.PostForm("reply"))
		title      = softTrunc(g.PostForm("title"), 100)
		content    = softTrunc(g.PostForm("content"), int(config.MaxContent))
		author     = softTrunc(g.PostForm("author"), 32)
		authorHash = authorNameToHash(author)
		tags       = splitTags(g.PostForm("tags"))
		deleted    = g.PostForm("delete") != ""
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
			a.Locked = locked
			m.UpdateArticle(a, a.Tags, false)
		}
		g.Redirect(302, redir)
		return
	}

	if a.Author != authorHash && !isAdmin(author) {
		g.Redirect(302, redir)
		return
	}

	if !deleted && !delimg {
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

	if a.Parent != 0 {
		g.Redirect(302, "/p/"+a.DisplayParentID())
	} else {
		if deleted {
			g.Redirect(302, "/")
		} else {
			g.Redirect(302, "/p/"+a.DisplayID())
		}
	}

}
