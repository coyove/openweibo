package action

import (
	"log"
	"net"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/coyove/iis/cmd/ch/view"
	"github.com/gin-gonic/gin"
)

var m *manager.Manager

func SetManager(mgr *manager.Manager) {
	m = mgr
}

func Edit(g *gin.Context) {
	if !g.GetBool("ip-ok") {
		view.Error(400, "guard/cooling-down", g)
		return
	}

	if _, ok := ident.ParseToken(g, g.PostForm("uuid")); !ok {
		view.Error(400, "guard/token-expired", g)
		return
	}

	var (
		eid         = ident.StringBytes(g.PostForm("reply"))
		title       = mv.SoftTrunc(g.PostForm("title"), 100)
		content     = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		author      = getAuthor(g)
		cat         = checkCategory(g.PostForm("cat"))
		locked      = g.PostForm("locked") != ""
		highlighted = g.PostForm("highlighted") != ""
		saged       = g.PostForm("saged") != ""
	)

	if !ident.IsAdmin(author) {
		g.Redirect(302, "/")
		return
	}

	a, err := m.Get(eid)
	if err != nil {
		g.Redirect(302, "/cat")
		return
	}

	redir := "/p/" + a.DisplayID()

	if locked != a.Locked || highlighted != a.Highlighted || saged != a.Saged {
		a.Locked, a.Highlighted, a.Saged = locked, highlighted, saged
		m.Update(a, a.Category)
		g.Redirect(302, redir)
		return
	}

	if a.Parent() == nil && len(title) == 0 {
		view.Error(400, "title/too-short", g)
		return
	}

	if len(content) == 0 {
		view.Error(400, "content/too-short", g)
		return
	}

	oldcat := a.Category
	a.Content, a.Category = content, cat

	if a.Parent() == nil {
		a.Title = title
	}

	if err := m.Update(a, oldcat); err != nil {
		log.Println(err)
		view.Error(500, "internal/error", g)
		return
	}

	g.Redirect(302, "/p/"+a.DisplayID())
}

func Delete(g *gin.Context) {
	if !g.GetBool("ip-ok") {
		view.Error(400, "guard/cooling-down", g)
		return
	}

	if _, ok := ident.ParseToken(g, g.PostForm("uuid")); !ok {
		view.Error(400, "guard/token-expired", g)
		return
	}

	var eid = ident.StringBytes(g.PostForm("reply"))
	var author = getAuthor(g)

	a, err := m.Get(eid)
	if err != nil {
		g.Redirect(302, "/cat")
		return
	}

	if a.Author != config.HashName(author) && !ident.IsAdmin(author) {
		log.Println(g.MustGet("ip").(net.IP), "tried to delete", a.ID)
		g.Redirect(302, "/p/"+a.DisplayID())
		return
	}

	if err := m.Delete(a); err != nil {
		log.Println(err)
		view.Error(500, "internal/error", g)
		return
	}

	if a.Parent() != nil {
		g.Redirect(302, "/p/"+a.DisplayParentID())
	} else {
		g.Redirect(302, "/cat")
	}
}

func Cookie(g *gin.Context) {
	if id := g.PostForm("id"); g.PostForm("clear") != "" || id == "" {
		g.SetCookie("id", "", -1, "", "", false, false)
	} else if g.PostForm("view") != "" {
		g.Redirect(302, "/cat/@@"+id)
		return
	} else {
		g.SetCookie("id", id, 86400*365, "", "", false, false)
	}
	g.Redirect(302, "/cookie")
}
