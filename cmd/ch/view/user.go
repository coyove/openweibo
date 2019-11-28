package view

import (
	"github.com/coyove/iis/cmd/ch/config"
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
		User      *mv.User
	}{
		Tags:      config.Cfg.Tags,
		EError:    g.Query("error"),
		RUsername: g.Query("username"),
		REmail:    g.Query("email"),
	}

	p.UUID, p.Challenge = ident.MakeToken(g)

	if u, ok := g.Get("user"); ok {
		p.User = u.(*mv.User)
	}

	g.HTML(200, "user.html", p)
}
