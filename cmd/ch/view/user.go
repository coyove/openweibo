package view

import (
	"image"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"strings"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/engine"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

var avatarPNG image.Image

func init() {
	f, err := os.Open("template/emoji/sprite-32.png")
	if err != nil {
		panic(err)
	}

	avatarPNG, _ = png.Decode(f)
}

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
		User     *mv.User
	}{
		EError:   g.Query("error"),
		ListType: g.Param("type"),
	}

	p.UUID, _ = ident.MakeToken(g)

	u, _ := g.Get("user")
	p.User, _ = u.(*mv.User)

	if p.User == nil {
		g.Redirect(302, "/user")
		return
	}

	switch p.ListType {
	case "blacklist":
		p.List, p.Next = m.GetFollowingList(p.User.BlockingChain, p.User, g.Query("n"), int(config.Cfg.PostsPerPage))
	case "followers":
		p.List, p.Next = m.GetFollowingList(p.User.FollowerChain, p.User, g.Query("n"), int(config.Cfg.PostsPerPage))
	default:
		p.List, p.Next = m.GetFollowingList(p.User.FollowingChain, p.User, g.Query("n"), int(config.Cfg.PostsPerPage))
	}

	g.HTML(200, "user_list.html", p)
}

func Avatar(g *gin.Context) {
	u, _ := m.GetUser(g.Param("id"))
	if u == nil {
		http.ServeFile(g.Writer, g.Request, "template/user.png")
	} else if !strings.HasPrefix(u.Avatar, "http") {
		h := 0
		for i := 0; i < len(u.ID); i++ {
			h = h*31 + int(u.ID[i])
		}
		// http.ServeFile(g.Writer, g.Request, "template/emoji/emoji"+strconv.Itoa(h%1832)+".png")
		h %= 1832
		i, j := h/43, h%43
		c := image.NewRGBA(image.Rect(0, 0, 32, 32))
		draw.Draw(c, c.Bounds(), avatarPNG, image.Pt(j*32, i*32), draw.Src)

		g.Writer.Header().Add("Content-Type", "image/png")
		png.Encode(g.Writer, c)
	} else {
		g.Redirect(302, u.Avatar)
	}
}
