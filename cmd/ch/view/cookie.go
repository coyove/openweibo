package view

import (
	"html/template"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/gin-gonic/gin"
)

func Cookie(g *gin.Context) {
	id, _ := g.Cookie("id")

	var p = struct {
		ID        string
		IsCrawler bool
		Config    template.HTML
		Tags      []string
	}{
		id,
		manager.IsCrawler(g),
		template.HTML(config.Cfg.PublicString),
		config.Cfg.Tags,
	}

	if ident.IsAdmin(g) {
		p.Config = template.HTML(config.Cfg.PrivateString)
	}

	g.HTML(200, "cookie.html", p)
}
