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
		g.AbortWithStatus(400)
		return
	}

	var (
		eid       = displayIDToObejctID(g.PostForm("reply"))
		title     = softTrunc(g.PostForm("title"), 64)
		content   = softTrunc(g.PostForm("content"), int(config.MaxContent))
		author    = authorNameToHash(g.PostForm("author"))
		tags      = splitTags(g.PostForm("tags"))
		deleted   = g.PostForm("delete") != ""
		announced = g.PostForm("announce") != ""
		locked    = g.PostForm("locked") != ""
	)

	a, err := m.GetArticle(eid)
	if err != nil {
		g.Redirect(302, "/")
		return
	}

	redir := "/p/" + a.DisplayID()

	if announced && !a.Announcement {
		if author == config.AdminNameHash {
			m.AnnounceArticle(eid)
		}
		g.Redirect(302, redir)
		return
	}

	if locked != a.Locked {
		if author == config.AdminNameHash {
			m.LockArticle(eid, locked)
		}
		g.Redirect(302, redir)
		return
	}

	if a.Author != author && author != config.AdminNameHash {
		g.Redirect(302, redir)
		return
	}

	if !deleted {
		if a.Parent == "" && len(title) < int(config.MinContent) {
			g.String(400, "Title Too Short")
			return
		}
		if len(content) < int(config.MinContent) {
			g.String(400, "Content Too Short")
			return
		}
		if a.Locked {
			g.String(400, "Post Locked")
			return
		}
	}

	if err := m.UpdateArticle(a.ID, deleted, title, content, tags); err != nil {
		log.Println(err)
		g.String(500, err.Error())
		return
	}

	if a.Parent != "" {
		g.Redirect(302, "/p/"+a.DisplayParentID())
	} else {
		g.Redirect(302, "/")
	}
}
