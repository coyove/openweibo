package view

import (
	"log"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/gin-gonic/gin"
)

func New(g *gin.Context) {
	var pl = struct {
		UUID     string
		Reply    string
		Abstract string
		Tags     []string
		IsAdmin  bool

		RTitle, RContent, RCat, EError string
	}{
		RTitle:   g.Query("title"),
		RContent: g.Query("content"),
		RCat:     g.Query("cat"),
		EError:   g.Query("error"),
		Tags:     config.Cfg.Tags,
		IsAdmin:  ident.IsAdmin(g),
	}

	pl.UUID, _ = ident.MakeToken(g)
	g.HTML(200, "newpost.html", pl)
}

func Edit(g *gin.Context) {
	var pl = struct {
		UUID           string
		Reply          string
		EError         string
		Tags           []string
		IsAdmin        bool
		IsAuthorBanned bool
		Article        ArticleView
	}{
		Reply:  g.Param("id"),
		Tags:   config.Cfg.Tags,
		EError: g.Query("error"),
	}

	u, ok := g.Get("user")
	if !ok {
		g.Redirect(302, "/cat")
		return
	}

	pl.UUID, _ = ident.MakeToken(g)
	pl.IsAdmin = u.(*mv.User).IsMod()

	a, err := m.Get(ident.ParseID(pl.Reply).String())
	if err != nil {
		log.Println(err)
		g.Redirect(302, "/cat")
		return
	}

	pl.Article.from(a, _showcontent)
	pl.IsAuthorBanned = m.IsBanned(a.Author)

	g.HTML(200, "editpost.html", pl)
}
