package view

import (
	"log"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/coyove/iis/cmd/ch/token"
	"github.com/gin-gonic/gin"
)

func New(g *gin.Context) {
	var pl = struct {
		UUID      string
		Reply     string
		Abstract  string
		Challenge string
		Tags      []string
		IsAdmin   bool

		RTitle, RAuthor, RContent, RCat, EError string
	}{
		RTitle:   g.Query("title"),
		RContent: g.Query("content"),
		RCat:     g.Query("cat"),
		RAuthor:  g.Query("author"),
		EError:   g.Query("error"),
		Tags:     config.Cfg.Tags,
		IsAdmin:  token.IsAdmin(g),
	}

	var answer [6]byte
	pl.UUID, answer = token.Make(g)
	pl.Challenge = token.GenerateCaptcha(answer)

	if pl.RAuthor == "" {
		pl.RAuthor, _ = g.Cookie("id")
	}

	g.HTML(200, "newpost.html", pl)
}

func Edit(g *gin.Context) {
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

	a, err := m.Get(ident.StringBytes(pl.Reply))
	if err != nil {
		log.Println(err)
		g.Redirect(302, "/cat")
		return
	}
	pl.Article = a

	g.HTML(200, "editpost.html", pl)
}
