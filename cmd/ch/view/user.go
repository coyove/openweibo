package view

import (
	"fmt"
	"image/jpeg"
	"net/http"
	"os"
	"strings"

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
	p.User = getUser(g)

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
		UUID:     ident.MakeUUID(g, nil),
		EError:   g.Query("error"),
		ListType: g.Param("type"),
	}

	p.You = getUser(g)
	if p.You == nil {
		g.Redirect(302, "/user")
		return
	}

	p.User, _ = m.GetUser(g.Param("uid"))
	if p.User == nil {
		p.User = p.You
	}

	var chain ident.ID
	switch p.ListType {
	case "blacklist":
		if p.User != p.You {
			g.Redirect(302, "/user/blacklist")
			return
		}
		chain = ident.NewID(ident.IDTagBlockChain).SetTag(p.User.ID)
	case "followers":
		chain = ident.NewID(ident.IDTagFollowerChain).SetTag(p.User.ID)
	default:
		chain = ident.NewID(ident.IDTagFollowChain).SetTag(p.User.ID)
	}
	p.List, p.Next = m.GetFollowingList(chain, g.Query("n"), int(config.Cfg.PostsPerPage))

	g.HTML(200, "user_list.html", p)
}

func Avatar(g *gin.Context) {
	id := strings.TrimSuffix(g.Param("id"), ".jpg")
	hash := (mv.User{ID: id}).IDHash()
	path := fmt.Sprintf("tmp/images/%d/%016x@%s", hash%1024, hash, id)

	if _, err := os.Stat(path); err == nil {
		http.ServeFile(g.Writer, g.Request, path)
	} else {
		img, _ := govatar.GenerateForUsername(govatar.MALE, id)
		g.Writer.Header().Add("Content-Type", "image/jpeg")
		g.Writer.Header().Add("Cache-Control", "public")
		jpeg.Encode(g.Writer, img, &jpeg.Options{Quality: 75})
	}
}

func UserLikes(g *gin.Context) {
	p := ArticlesTimelineView{
		IsUserLikeTimeline: true,
		MediaOnly:          g.Query("media") != "",
		ReplyView:          makeReplyView(g, ""),
		You:                getUser(g),
	}

	if p.You == nil {
		g.Redirect(302, "/user")
		return
	}

	p.User, _ = m.GetUser(g.Param("uid"))
	if p.User == nil {
		p.User = p.You
	}

	var cursor string
	if pa, _ := m.GetArticle(ident.NewID(ident.IDTagLikeChain).SetTag(p.User.ID).String()); pa != nil {
		cursor = pa.PickNextID(p.MediaOnly)
	}

	a, next := m.WalkLikes(p.MediaOnly, int(config.Cfg.PostsPerPage), cursor)
	fromMultiple(&p.Articles, a, 0, getUser(g))
	p.Next = next

	g.HTML(200, "timeline.html", p)
}
