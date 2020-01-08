package view

import (
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/engine"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

var m *manager.Manager

type ArticlesTimelineView struct {
	Articles              []ArticleView
	Next                  string
	Tag                   string
	PostsUnderTag         int32
	IsInbox               bool
	IsUserTimeline        bool
	IsUserLikeTimeline    bool
	IsTagTimelineFollowed bool
	IsTagTimeline         bool
	ShowNewPost           bool
	MediaOnly             bool
	User                  *mv.User
	You                   *mv.User
	ReplyView             ReplyView
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
		MediaOnly:     g.Query("media") != "",
		ReplyView:     makeReplyView(g, ""),
	}

	if pl.You != nil {
		pl.IsTagTimelineFollowed = m.IsFollowing(pl.You.ID, "#"+pl.Tag)
	}

	a, _ := m.GetArticle(ident.NewID(ident.IDTagTag).SetTag(pl.Tag).String())
	if a != nil {
		pl.PostsUnderTag = int32(a.Replies)
	}

	a2, next := m.WalkMulti(pl.MediaOnly, int(config.Cfg.PostsPerPage), ident.NewID(ident.IDTagTag).SetTag(pl.Tag))
	fromMultiple(&pl.Articles, a2, _Blank, getUser(g))

	pl.Next = ident.CombineIDs(nil, next...)
	g.HTML(200, "timeline.html", pl)
}

func Timeline(g *gin.Context) {
	pl := ArticlesTimelineView{
		ReplyView: makeReplyView(g, ""),
		You:       getUser(g),
		MediaOnly: g.Query("media") != "",
	}

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
		if pl.You != nil && pl.You.ID != pl.User.ID {
			pl.User.SetIsFollowing(m.IsFollowing(pl.You.ID, uid))
			pl.User.SetIsBlocking(m.IsBlocking(pl.You.ID, uid))
			pl.User.SetIsNotYou(true)
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
		list, next := m.GetFollowingList(ident.NewID(ident.IDTagFollowChain).SetTag(pl.User.ID), start, 1e6)
		for _, id := range list {
			if id.Followed {
				if strings.HasPrefix(id.ID, "#") {
					cursors = append(cursors, ident.NewID(ident.IDTagTag).SetTag(id.ID[1:]))
				} else {
					cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(id.ID))
				}
			}
		}
		pendingFCursor = next
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

	a, next := m.WalkMulti(pl.MediaOnly, int(config.Cfg.PostsPerPage), cursors...)
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
	if g.PostForm("likes") == "true" {
		a, next := m.WalkLikes(g.PostForm("media") == "true", int(config.Cfg.PostsPerPage), g.PostForm("cursors"))
		fromMultiple(&articles, a, _Blank, getUser(g))
		p.Next = next
	} else if g.PostForm("reply") == "true" {
		a, next := m.WalkReply(int(config.Cfg.PostsPerPage), g.PostForm("cursors"))
		fromMultiple(&articles, a, _NoMoreParent|_ShowAvatar, getUser(g))
		p.Next = next
	} else {
		cursors, payload := ident.SplitIDs(g.PostForm("cursors"))

		var pendingFCursor string
		if len(payload) > 0 {
			list, next := m.GetFollowingList(ident.ID{}, string(payload), 1e6)
			// log.Println(list, next, string(payload))
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
			pendingFCursor = next
		}

		a, next := m.WalkMulti(g.PostForm("media") == "true", int(config.Cfg.PostsPerPage), cursors...)
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

func APIReplies(g *gin.Context) {
	var pl ArticleRepliesView
	var pid = g.Param("parent")

	parent, err := m.GetArticle(pid)
	if err != nil || parent.ID == "" {
		g.Status(404)
		log.Println(pid, err)
		return
	}

	pl.ParentArticle.from(parent, 0, getUser(g))
	pl.ReplyView = makeReplyView(g, pid)
	pl.FrameID = strconv.FormatInt(time.Now().Unix(), 16)

	if u, ok := g.Get("user"); ok {
		if m.IsBlocking(pl.ParentArticle.Author.ID, u.(*mv.User).ID) {
			g.Status(404)
			return
		}
	}

	a, next := m.WalkReply(int(config.Cfg.PostsPerPage), parent.ReplyChain)
	fromMultiple(&pl.Articles, a, _NoMoreParent|_ShowAvatar, getUser(g))
	pl.Next = next

	g.Writer.Header().Add("X-Reply", "true")
	g.HTML(200, "post.html", pl)
}
