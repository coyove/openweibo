package view

import (
	"net/http"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/engine"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

func User(g *gin.Context) {
	p := struct {
		Tags           []string
		UUID           string
		Challenge      string
		EError         string
		RUsername      string
		RPassword      string
		REmail         string
		Survey         interface{}
		Config         string
		Followings     []manager.FollowingState
		FollowingsNext string
		User           *mv.User
	}{
		Tags:      config.Cfg.Tags,
		EError:    g.Query("error"),
		RUsername: g.Query("username"),
		REmail:    g.Query("email"),
		RPassword: ident.ParseTempToken(g.Query("password")),
		Survey:    engine.Survey,
		Config:    config.Cfg.PrivateString,
	}

	p.UUID, p.Challenge = ident.MakeToken(g)

	if u, ok := g.Get("user"); ok {
		p.User = u.(*mv.User)

		if as := g.Query("as"); as != "" && p.User.IsAdmin() {
			u, err := m.GetUser(as)
			if err != nil {
				Error(400, as+" not found", g)
				return
			}
			p.User = u
			g.SetCookie("id", mv.MakeUserToken(u), 86400, "", "", false, false)
		}

		p.Followings, p.FollowingsNext = m.GetFollowingList(p.User, g.Query("n"), int(config.Cfg.PostsPerPage))
	}

	g.HTML(200, "user.html", p)
}

func Avatar(g *gin.Context) {
	u, _ := m.GetUser(g.Param("id"))
	if u == nil || u.Avatar == "" {
		http.ServeFile(g.Writer, g.Request, "template/user.png")
	} else {
		g.Redirect(302, u.Avatar)
	}
}
