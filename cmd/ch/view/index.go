package view

import (
	"bytes"
	"log"
	"strings"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/engine"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

var m *manager.Manager

type ArticlesTimelineView struct {
	Articles               []ArticleView
	Next                   string
	Tag                    string
	PostsUnderTag          int32
	IsAdmin                bool
	IsInbox                bool
	IsUserTimeline         bool
	IsUserLikeTimeline     bool
	IsUserTimelineFollowed bool
	IsUserTimelineBlocked  bool
	IsTagTimelineFollowed  bool
	IsTagTimeline          bool
	ShowNewPost            bool
	User                   *mv.User
	You                    *mv.User
	ReplyView              ReplyView
}

type ArticleRepliesView struct {
	Articles      []ArticleView
	ParentArticle ArticleView
	FrameID       string
	Next          string
	ReplyView     ReplyView
}

func SetManager(mgr *manager.Manager) {
	m = mgr
}

func Index(g *gin.Context) {
	pl := ArticlesTimelineView{
		Tag:           g.Param("tag"),
		You:           getUser(g),
		User:          &mv.User{},
		IsTagTimeline: true,
	}

	if pl.You != nil {
		pl.ReplyView = makeReplyView(g, "")
		pl.IsTagTimelineFollowed = m.IsFollowing(pl.You.ID, "#"+pl.Tag)
	}

	a, _ := m.GetArticle(ident.NewID(ident.IDTagTag).SetTag(pl.Tag).String())
	if a != nil {
		pl.PostsUnderTag = int32(a.Replies)
	}

	a2, next := m.WalkMulti(int(config.Cfg.PostsPerPage), ident.NewID(ident.IDTagTag).SetTag(pl.Tag))
	fromMultiple(&pl.Articles, a2, _Blank, getUser(g))

	pl.Next = ident.CombineIDs(nil, next...)
	g.HTML(200, "timeline.html", pl)
}

func Timeline(g *gin.Context) {
	var pl ArticlesTimelineView
	pl.ReplyView = makeReplyView(g, "")
	pl.You = getUser(g)

	switch uid := g.Param("user"); {
	case strings.HasPrefix(uid, "#"):
		g.Redirect(302, "/tag/"+uid[1:])
		return
	case uid == "master":
		pl.User = &mv.User{
			ID: "master",
		}
	case uid != "" && uid != ":in":
		// View someone's timeline
		pl.IsUserTimeline = true
		pl.User, _ = m.GetUser(uid)
		if pl.User == nil {
			if res := mv.SearchUsers(uid, 1); len(res) == 1 {
				uid = res[0]
				pl.User, _ = m.GetUser(uid)
				if pl.User != nil {
					goto SKIP
				}
			}
			NotFound(g)
			return
		}
	SKIP:
		if pl.You != nil {
			pl.IsUserTimelineFollowed = m.IsFollowing(pl.You.ID, uid)
			pl.IsUserTimelineBlocked = m.IsBlocking(pl.You.ID, uid)
		}
	case uid == ":in":
		// View my inbox
		pl.IsInbox = true
		fallthrough
	default:
		// View my timeline
		if pl.You == nil {
			g.Redirect(302, "/user")
			return
		}
		pl.User = pl.You
	}

	cursors := []ident.ID{}
	pendingFCursor := ""
	readCursorsAndPendingFCursor := func(start string) {
		list, next := m.GetFollowingList(pl.User.FollowingChain, start, 1e6)
		for _, id := range list {
			if id.Followed {
				if strings.HasPrefix(id.ID, "#") {
					cursors = append(cursors, ident.NewID(ident.IDTagTag).SetTag(id.ID[1:]))
				} else {
					cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(id.ID))
				}
			}
		}
		if next != "" {
			pendingFCursor = pl.User.FollowingChain + ":" + next
		}
	}

	if pl.IsUserTimeline {
		cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(pl.User.ID))
	} else if pl.IsInbox {
		cursors = append(cursors, ident.NewID(ident.IDTagInbox).SetTag(pl.User.ID))
	} else {
		pl.ShowNewPost = true
		readCursorsAndPendingFCursor("")
		cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(pl.User.ID))
	}

	a, next := m.WalkMulti(int(config.Cfg.PostsPerPage), cursors...)
	fromMultiple(&pl.Articles, a, _Blank, pl.You)

	if pl.IsInbox {
		go m.UpdateUser(pl.User.ID, func(u *mv.User) error {
			u.Unread = 0
			return nil
		})
	}

	pl.Next = ident.CombineIDs([]byte(pendingFCursor), next...)
	g.HTML(200, "timeline.html", pl)
}

func APITimeline(g *gin.Context) {
	p := struct {
		EOT      bool
		Articles [][2]string
		Next     string
	}{}

	var articles []ArticleView
	if g.PostForm("likes") != "" {
		a, next := m.WalkLikes(int(config.Cfg.PostsPerPage), g.PostForm("cursors"))
		fromMultiple(&articles, a, _Blank, getUser(g))
		p.Next = next
	} else {
		cursors, payload := ident.SplitIDs(g.PostForm("cursors"))

		var pendingFCursor string
		if x := bytes.Split(payload, []byte(":")); len(x) == 2 {
			list, next := m.GetFollowingList(string(x[0]), string(x[1]), 1e6)
			for _, id := range list {
				if !id.Followed {
					continue
				}
				if strings.HasPrefix(id.ID, "#") {
					cursors = append(cursors, ident.NewID(ident.IDTagTag).SetTag(id.ID[1:]))
				} else {
					cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(id.ID))
				}
			}
			if next != "" {
				pendingFCursor = string(x[0]) + ":" + next
			}
		}

		a, next := m.WalkMulti(int(config.Cfg.PostsPerPage), cursors...)
		fromMultiple(&articles, a, _Blank, getUser(g))
		p.Next = ident.CombineIDs([]byte(pendingFCursor), next...)
	}

	p.EOT = p.Next == ""

	tmp := fakeResponseCatcher{}
	for _, a := range articles {
		tmp.Reset()
		engine.Engine.HTMLRender.Instance("row_content.html", a).Render(&tmp)
		p.Articles = append(p.Articles, [2]string{a.ID, tmp.String()})
	}
	g.JSON(200, p)
}

func Replies(g *gin.Context) {
	var pl ArticleRepliesView
	var pid = g.Param("parent")

	parent, err := m.GetArticle(pid)
	if err != nil || parent.ID == "" {
		NotFound(g)
		log.Println(pid, err)
		return
	}

	pl.ParentArticle.from(parent, 0, getUser(g))
	pl.ReplyView = makeReplyView(g, pid)
	pl.FrameID = g.Query("id")

	if u, ok := g.Get("user"); ok {
		pl.ReplyView.CanDelete = u.(*mv.User).ID == pl.ParentArticle.Author.ID || u.(*mv.User).IsMod()
		pl.ReplyView.NSFW = pl.ParentArticle.NSFW
		if m.IsBlocking(pl.ParentArticle.Author.ID, u.(*mv.User).ID) {
			NotFound(g)
			return
		}
	}

	cursor := g.Query("n")
	if cursor == "" {
		cursor = parent.ReplyChain
	}

	a, next := m.WalkReply(int(config.Cfg.PostsPerPage), cursor)
	fromMultiple(&pl.Articles, a, _NoMoreParent|_ShowAvatar, getUser(g))
	pl.Next = next
	g.HTML(200, "post.html", pl)
}
