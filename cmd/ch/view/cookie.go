package view

import (
	"html/template"
	"net/url"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

func Home(g *gin.Context) {
	id, _ := g.Cookie("id")

	var p = struct {
		ID        string
		IsCrawler bool
		IsAdmin   bool
		Config    template.HTML
		Tags      []string
	}{
		id,
		manager.IsCrawler(g),
		ident.IsAdmin(g),
		template.HTML(config.Cfg.PublicString),
		config.Cfg.Tags,
	}

	if ident.IsAdmin(g) {
		p.Config = template.HTML(config.Cfg.PrivateString)
	}

	g.HTML(200, "home.html", p)
}

func Search(g *gin.Context) {
	q := g.Query("q")
	p := struct {
		Stat  interface{}
		Count int
	}{}
	p.Stat, p.Count = mv.SearchStat()

	if q == "" {
		g.HTML(200, "search.html", p)
	} else {
		g.Redirect(302, "/cat/search:"+url.PathEscape(q))
	}
}
