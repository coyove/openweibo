package view

import (
	"image/jpeg"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/engine"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
	"github.com/o1egl/govatar"
)

func User(g *gin.Context) {
	p := struct {
		UUID       string
		Challenge  string
		EError     string
		RUsername  string
		RPassword  string
		REmail     string
		LoginError string
		Survey     interface{}
		Config     string
		User       *mv.User
	}{
		EError:     g.Query("error"),
		LoginError: g.Query("login-error"),
		RUsername:  g.Query("username"),
		REmail:     g.Query("email"),
		RPassword:  ident.ParseTempToken(g.Query("password")),
		Survey:     engine.Survey,
		Config:     config.Cfg.PrivateString,
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
		UUID     string
		List     []manager.FollowingState
		EError   string
		Next     string
		ListType string
		You      *mv.User
		User     *mv.User
	}{
		EError:   g.Query("error"),
		ListType: g.Param("type"),
	}

	p.UUID, _ = ident.MakeToken(g)
	p.You = getUser(g)
	if p.You == nil {
		g.Redirect(302, "/user")
		return
	}

	p.User, _ = m.GetUser(g.Param("uid"))
	if p.User == nil {
		p.User = p.You
	}

	switch p.ListType {
	case "blacklist":
		if p.User != p.You {
			g.Redirect(302, "/user/blacklist")
			return
		}
		p.List, p.Next = m.GetFollowingList(p.User.BlockingChain, p.User, g.Query("n"), int(config.Cfg.PostsPerPage))
	case "followers":
		p.List, p.Next = m.GetFollowingList(p.User.FollowerChain, p.User, g.Query("n"), int(config.Cfg.PostsPerPage))
	default:
		p.List, p.Next = m.GetFollowingList(p.User.FollowingChain, p.User, g.Query("n"), int(config.Cfg.PostsPerPage))
	}

	g.HTML(200, "user_list.html", p)
}

func Avatar(g *gin.Context) {
	img, _ := govatar.GenerateForUsername(govatar.MALE, g.Param("id"))
	g.Writer.Header().Add("Content-Type", "image/jpeg")
	g.Writer.Header().Add("Cache-Control", "public")
	jpeg.Encode(g.Writer, img, &jpeg.Options{Quality: 75})
}
