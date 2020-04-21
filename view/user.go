package view

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/middleware"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
	"github.com/nullrocks/identicon"
)

func User(g *gin.Context) {
	p := struct {
		UUID      string
		Challenge string
		Survey    interface{}
		User      *model.User
	}{
		Survey: middleware.Survey,
	}

	p.UUID, p.Challenge = ik.MakeToken(g)
	p.User = getUser(g)
	if p.User != nil {
		p.User.SetShowList('S')
	}
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

	p.User, _ = dal.GetUser(g.Param("uid"))
	if p.User == nil {
		p.User = p.You
	}

	p.User.Buildup(p.You)

	switch p.ListType {
	case "blacklist":
		if p.User != p.You {
			g.Redirect(302, "/user/blacklist")
			return
		}
		p.List, p.Next = dal.GetRelationList(p.User, ik.NewID(ik.IDBlacklist, p.User.ID), g.Query("n"), int(common.Cfg.PostsPerPage))
		p.User.SetShowList('b')
	case "followers":
		p.List, p.Next = dal.GetRelationList(p.User, ik.NewID(ik.IDFollower, p.User.ID), g.Query("n"), int(common.Cfg.PostsPerPage))
		p.User.SetShowList('s')
	case "twohops":
		if p.You.ID == p.User.ID {
			g.Redirect(302, "/user/followings")
			return
		}
		p.List, p.Next = dal.GetCommonFollowingList(p.You.ID, p.User.ID, g.Query("n"), int(common.Cfg.PostsPerPage))
		p.User.SetShowList('r')
	default:
		p.List, p.Next = dal.GetFollowingList(ik.NewID(ik.IDFollowing, p.User.ID), g.Query("n"), int(common.Cfg.PostsPerPage), true)
		p.User.SetShowList('f')
	}

	g.HTML(200, "user_list.html", p)
}

var ig, _ = identicon.New("github", 5, 3)

func Avatar(g *gin.Context) {
	id := strings.TrimSuffix(g.Param("id"), ".jpg")
	hash := (model.User{ID: id}).IDHash()
	path := fmt.Sprintf("tmp/images/%d/%016x@%s", hash%1024, hash, id)

	if _, err := os.Stat(path); err == nil {
		http.ServeFile(g.Writer, g.Request, path)
	} else {
		ii, err := ig.Draw("iis" + id)
		if err != nil {
			log.Println(err)
			g.Status(404)
			return
		}
		g.Writer.Header().Add("Content-Type", "image/jpeg")
		g.Writer.Header().Add("Cache-Control", "public")
		ii.Jpeg(100, 80, g.Writer)
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

	if uid := g.Param("uid"); uid != "master" {
		p.User, _ = dal.GetUser(uid)
		if p.User == nil {
			p.User = p.You
		}
	} else {
		p.User = p.You
	}

	var cursor string
	if pa, _ := dal.GetArticle(ik.NewID(ik.IDLike, p.User.ID).String()); pa != nil {
		cursor = pa.PickNextID(p.MediaOnly)
	}

	a, next := dal.WalkLikes(p.MediaOnly, int(common.Cfg.PostsPerPage), cursor)
	fromMultiple(&p.Articles, a, 0, getUser(g))
	p.Next = next

	g.HTML(200, "timeline.html", p)
}

func APIGetUserInfoBox(g *gin.Context) {
	u, _ := dal.GetUserWithSettings(g.Param("id"))
	if u == nil {
		g.String(200, "internal/error")
		return
	}

	if you := getUser(g); you != nil {
		u.Buildup(you)
	}

	s := middleware.RenderTemplateString("user_public.html", u)
	g.String(200, "ok:"+s)
}
