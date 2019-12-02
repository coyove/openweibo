package action

import (
	"log"
	"net"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/manager"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/gin-gonic/gin"
)

var m *manager.Manager

func SetManager(mgr *manager.Manager) {
	m = mgr
}

func Edit(g *gin.Context) {
	redir := func(a, b string) {
		g.Redirect(302, "/edit/"+g.PostForm("reply")+EncodeQuery(a, b))
	}

	if ret := checkToken(g); ret != "" {
		redir("error", ret)
		return
	}

	var (
		title       = mv.SoftTrunc(g.PostForm("title"), 100)
		content     = mv.SoftTrunc(g.PostForm("content"), int(config.Cfg.MaxContent))
		cat         = checkCategory(g.PostForm("cat"))
		locked      = g.PostForm("locked") != ""
		highlighted = g.PostForm("highlighted") != ""
	)

	u, ok := g.Get("user")
	if !ok {
		g.Redirect(302, "/")
		return
	}

	if !u.(*mv.User).IsMod() {
		g.Redirect(302, "/")
		return
	}

	a, err := m.Get(g.PostForm("reply"))
	if err != nil {
		g.Redirect(302, "/cat")
		return
	}

	if locked != a.Locked || highlighted != a.Highlighted {
		a.Locked, a.Highlighted = locked, highlighted
		m.Update(a)
		g.Redirect(302, "/p/"+a.ID)
		return
	}

	if p, _ := a.Parent(); p == "" && len(title) == 0 {
		redir("error", "title/too-short")
		return
	}

	if len(content) == 0 {
		redir("error", "content/too-short")
		return
	}

	oldcat := a.Category
	a.Content, a.Category = content, cat

	if p, _ := a.Parent(); p == "" {
		a.Title = title
	}

	if err := m.Update(a, oldcat); err != nil {
		log.Println(err)
		redir("error", "internal/error")
		return
	}

	g.Redirect(302, "/p/"+a.ID)
}

func Delete(g *gin.Context) {
	redir := func(a, b string) {
		g.Redirect(302, "/edit/"+g.PostForm("reply")+EncodeQuery(a, b))
	}

	if ret := checkToken(g); ret != "" {
		redir("error", ret)
		return
	}

	u, ok := g.Get("user")
	if !ok {
		redir("error", "internal/error")
		return
	}

	a, err := m.Get(g.PostForm("reply"))
	if err != nil {
		g.Redirect(302, "/cat")
		return
	}

	if a.Author != u.(*mv.User).ID && !u.(*mv.User).IsMod() {
		log.Println(g.MustGet("ip").(net.IP), "tried to delete", a.ID)
		g.Redirect(302, "/p/"+a.ID)
		return
	}

	if err := m.Delete(a); err != nil {
		log.Println(err)
		redir("error", "internal/error")
		return
	}

	if p, _ := a.Parent(); p != "" {
		g.Redirect(302, "/p/"+p)
	} else {
		g.Redirect(302, "/cat")
	}
}
