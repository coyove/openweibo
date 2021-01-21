package handler

import (
	"fmt"
	"net/http"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/common/avatar"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/middleware"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func User(g *gin.Context) {
	m, _ := g.Cookie("mode")
	p := struct {
		UUID        string
		Challenge   string
		User        *model.User
		SiteKey     string
		DarkCaptcha bool
	}{
		SiteKey:     common.Cfg.HCaptchaSiteKey,
		DarkCaptcha: m == "dark",
	}

	if g.Query("myip") != "" { // a handy service
		g.String(200, g.ClientIP())
		return
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
		List     []dal.FollowingState
		Next     string
		ListType string
		You      *model.User
		User     *model.User
		API      bool
	}{
		API:      g.Request.Method == "POST",
		ListType: g.Param("type"),
		You:      getUser(g),
		User:     getUser(g),
	}

	if p.You == nil {
		redirectVisitor(g)
		return
	}

	p.User, _ = dal.GetUser(g.Param("uid"))
	if p.User == nil {
		NotFound(g)
		return
	}

	if !checkFollowApply(g, p.User, p.You) {
		NotFound(g)
		return
	}

	p.User.Buildup(p.You)
	next := g.PostForm("next")
	if next == "" {
		next = g.Query("n")
	}

	switch p.ListType {
	case "blacklist":
		if p.User.ID != p.You.ID {
			NotFound(g)
			return
		}
		p.List, p.Next = dal.GetRelationList(p.User, ik.NewID(ik.IDBlacklist, p.User.ID), next, int(common.Cfg.PostsPerPage))
	case "followers":
		p.List, p.Next = dal.GetRelationList(p.User, ik.NewID(ik.IDFollower, p.User.ID), next, int(common.Cfg.PostsPerPage))
	case "twohops":
		if p.You.ID == p.User.ID {
			g.Redirect(302, "/t")
			return
		}
		p.List, p.Next = dal.GetCommonFollowingList(p.You.ID, p.User.ID, next, int(common.Cfg.PostsPerPage))
	default:
		p.List, p.Next = dal.GetFollowingList(ik.NewID(ik.IDFollowing, p.User.ID), next, int(common.Cfg.PostsPerPage), true)
	}

	if p.API {
		g.Writer.Header().Add("X-Next", p.Next)
		g.String(200, middleware.RenderTemplateString("user_list.html", p))
	} else {
		g.HTML(200, "user_list.html", p)
	}
}

// var ig, _ = identicon.New("github", 5, 3)

func Avatar(g *gin.Context) {
	id := g.Param("id")
	if len(id) == 0 {
		g.Status(404)
		return
	}

	g.Writer.Header().Add("Cache-Control", "max-age=8640000")
	if g.Query("q") == "0" {
		g.Writer.Header().Add("Content-Type", "image/jpeg")
		avatar.Create(g.Writer, id)
		return
	}

	hash := (model.User{ID: id}).IDHash()
	path := fmt.Sprintf("tmp/images/%016x@%s", hash, id)

	http.ServeFile(g.Writer, g.Request, path)
}

func UserLikes(g *gin.Context) {
	p := ArticlesTimelineView{
		IsUserLikeTimeline: true,
		MediaOnly:          g.Query("media") != "",
		You:                getUser(g),
	}

	if p.You == nil {
		redirectVisitor(g)
		return
	}

	if uid := g.Param("uid"); uid != "master" {
		p.User, _ = dal.GetUser(uid)
		if p.User == nil {
			p.User = p.You
		} else {
			if p.User.HideLikes == 1 {
				NotFound(g)
				return
			}

			if !checkFollowApply(g, p.User, p.You) {
				return
			}
		}
	} else {
		p.User = p.You
	}

	var cursor string
	if pa, _ := dal.GetArticle(ik.NewID(ik.IDLike, p.User.ID).String()); pa != nil {
		cursor = pa.PickNextID(p.MediaOnly)
	}

	a, next := dal.WalkLikes(p.MediaOnly, int(common.Cfg.PostsPerPage), cursor)
	fromMultiple(g, &p.Articles, a, 0, getUser(g))
	p.Next = next

	g.HTML(200, "timeline.html", p)
}

func APIGetUserInfoBox(g *gin.Context) {
	you := getUser(g)
	id := g.Param("id")
	u, _ := dal.GetUser(id)
	// throw(u, "user_not_found_by_id")
	if u == nil {
		u = &model.User{
			ID: id,
		}
		u.SetShowList(255)
		okok(g, middleware.RenderTemplateString("user_public.html", u))
		return
	}

	if you != nil {
		u.Buildup(you)
	}

	okok(g, middleware.RenderTemplateString("user_public.html", u))
}

func UserSecurity(g *gin.Context) {
	if getUser(g) == nil {
		NotFound(g)
		return
	}
	g.HTML(200, g.Request.URL.Path[1:]+".html", getUser(g))
}
