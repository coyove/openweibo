package view

import (
	"fmt"
	"image/jpeg"
	"net/http"
	"os"
	"strings"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/middleware"
	"github.com/coyove/iis/model"
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
		User       *model.User
	}{
		EError:     g.Query("error"),
		LoginError: g.Query("login-error"),
		RUsername:  g.Query("username"),
		REmail:     g.Query("email"),
		RPassword:  ik.ParseTempToken(g.Query("password")),
		Survey:     middleware.Survey,
		Config:     common.Cfg.PrivateString,
	}

	p.UUID, p.Challenge = ik.MakeToken(g)
	p.User = getUser(g)

	g.HTML(200, "user.html", p)
}

func UserList(g *gin.Context) {
	p := struct {
		UUID     string
		List     []dal.FollowingState
		EError   string
		Next     string
		ListType string
		You      *model.User
		User     *model.User
	}{
		UUID:     ik.MakeUUID(g, nil),
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

	var chain ik.ID
	switch p.ListType {
	case "blacklist":
		if p.User != p.You {
			g.Redirect(302, "/user/blacklist")
			return
		}
		chain = ik.NewID(ik.IDTagBlockChain).SetTag(p.User.ID)
	case "followers":
		chain = ik.NewID(ik.IDTagFollowerChain).SetTag(p.User.ID)
	default:
		chain = ik.NewID(ik.IDTagFollowChain).SetTag(p.User.ID)
	}
	p.List, p.Next = m.GetFollowingList(chain, g.Query("n"), int(common.Cfg.PostsPerPage))

	g.HTML(200, "user_list.html", p)
}

func Avatar(g *gin.Context) {
	id := strings.TrimSuffix(g.Param("id"), ".jpg")
	hash := (model.User{ID: id}).IDHash()
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
	if pa, _ := m.GetArticle(ik.NewID(ik.IDTagLikeChain).SetTag(p.User.ID).String()); pa != nil {
		cursor = pa.PickNextID(p.MediaOnly)
	}

	a, next := m.WalkLikes(p.MediaOnly, int(common.Cfg.PostsPerPage), cursor)
	fromMultiple(&p.Articles, a, 0, getUser(g))
	p.Next = next

	g.HTML(200, "timeline.html", p)
}
