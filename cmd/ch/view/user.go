package view

import (
	"net/http"
	"strings"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/engine"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

func User(g *gin.Context) {
	p := struct {
		Tags      []string
		UUID      string
		Challenge string
		EError    string
		RUsername string
		RPassword string
		REmail    string
		Survey    interface{}
		Config    string
		User      *mv.User
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

	}

	g.HTML(200, "user.html", p)
}

func UserList(g *gin.Context) {
	p := struct {
		UUID        string
		List        []manager.FollowingState
		EError      string
		Next        string
		IsBlacklist bool
		User        *mv.User
	}{
		EError:      g.Query("error"),
		IsBlacklist: g.Param("type") == "blacklist",
	}

	p.UUID, _ = ident.MakeToken(g)

	u, _ := g.Get("user")
	p.User, _ = u.(*mv.User)

	if p.User == nil {
		g.Redirect(302, "/user")
		return
	}

	p.List, p.Next = m.GetFollowingList(p.IsBlacklist, p.User, g.Query("n"), int(config.Cfg.PostsPerPage))

	g.HTML(200, "user_list.html", p)
}

func Avatar(g *gin.Context) {
	u, _ := m.GetUser(g.Param("id"))
	if u == nil || !strings.HasPrefix(u.Avatar, "http") {
		http.ServeFile(g.Writer, g.Request, "template/user.png")
	} else {
		g.Redirect(302, u.Avatar)
	}
}
