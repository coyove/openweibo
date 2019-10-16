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
		eid         = ident.GDecryptString(g, g.PostForm("reply")).String()
		title       = mv.SoftTrunc(g.PostForm("title"), 100)
		content     = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		author      = getAuthor(g)
		cat         = checkCategory(g.PostForm("cat"))
		locked      = g.PostForm("locked") != ""
		highlighted = g.PostForm("highlighted") != ""
		saged       = g.PostForm("saged") != ""
		banID       = g.PostForm("banned") != ""
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

	redir := "/p/" + ident.GDecryptString(g, a.ID).String()

	if isBan := m.IsBanned(a.Author); isBan != banID {
		if isBan {
			err = m.Unban(a.Author)
		} else {
			err = m.Ban(a.Author)
		}
		if err != nil {
			view.Error(400, err.Error(), g)
		} else {
			g.Redirect(302, redir)
		}
		return
	}

	if locked != a.Locked || highlighted != a.Highlighted || saged != a.Saged {
		a.Locked, a.Highlighted, a.Saged = locked, highlighted, saged
		m.Update(a)
		g.Redirect(302, redir)
		return
	}

	if a.Parent() == "" && len(title) == 0 {
		view.Error(400, "title/too-short", g)
		return
	}

	if len(content) == 0 {
		view.Error(400, "content/too-short", g)
		return
	}

	a.Content, a.Category = content, cat

	if a.Parent() == "" {
		a.Title = title
	}

	if err := m.Update(a); err != nil {
		log.Println(err)
		view.Error(500, "internal/error", g)
		return
	}

	g.Redirect(302, redir)
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

	var eid = ident.GDecryptString(g, g.PostForm("reply")).String()
	var author = getAuthor(g)

	a, err := m.Get(eid)
	if err != nil {
		g.Redirect(302, "/cat")
		return
	}

	if a.Author != config.HashName(author) && !ident.IsAdmin(author) {
		log.Println(g.MustGet("ip").(net.IP), "tried to delete", a.ID)
		g.Redirect(302, "/p/"+ident.GDecryptString(g, a.ID).String())
		return
	}

	if err := m.Delete(a); err != nil {
		log.Println(err)
		view.Error(500, "internal/error", g)
		return
	}

	if a.Parent() != "" {
		g.Redirect(302, "/p/"+ident.GEncryptString(g, ident.ParseIDString(nil, a.Parent())))
	} else {
		g.Redirect(302, "/cat")
	}
}
