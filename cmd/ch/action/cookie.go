package action

import (
	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/gin-gonic/gin"
)

func Cookie(g *gin.Context) {
	if ident.IsAdmin(g) && g.PostForm("needid") != "" {
		config.Cfg.NeedID = !config.Cfg.NeedID
		config.RegenConfigString()
	} else if g.PostForm("noobfs") != "" {
		if manager.IsCrawler(g) {
			g.SetCookie("crawler", "", -1, "", "", false, false)
		} else {
			g.SetCookie("crawler", "1", 86400*365, "", "", false, false)
		}
	} else if id := g.PostForm("id"); g.PostForm("clear") != "" || id == "" {
		g.SetCookie("id", "", -1, "", "", false, false)
	} else if g.PostForm("view") != "" {
		g.Redirect(302, "/cat/@@"+id)
		return
	} else if len(id) > 3 {
		g.SetCookie("id", id, 86400*365, "", "", false, false)
	}
	g.Redirect(302, "/")
}
