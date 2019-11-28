package view

import (
	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/engine"
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/gin-gonic/gin"
)

func User(g *gin.Context) {
	p := struct {
		Tags      []string
		UUID      string
		Challenge string
		EError    string
		RUsername string
		REmail    string
		Survey    interface{}
		Config    string
		User      *mv.User
	}{
		Tags:      config.Cfg.Tags,
		EError:    g.Query("error"),
		RUsername: g.Query("username"),
		REmail:    g.Query("email"),
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
