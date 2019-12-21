package view

import (
	"encoding/base64"
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
	IsUserTimelineFollowed bool
	IsUserTimelineBlocked  bool
	IsTagTimelineFollowed  bool
	IsGlobalTimeline       bool
	IsTagTimeline          bool
	ShowNewPost            bool
	User                   *mv.User
	You                    *mv.User
	ReplyView              ReplyView
}

type ArticleRepliesView struct {
	Articles      []ArticleView
	ParentArticle ArticleView
	Next          string
	ReplyView     ReplyView
}

func SetManager(mgr *manager.Manager) {
	m = mgr
}

func Index(g *gin.Context) {
	var pl ArticlesTimelineView
	var cursor ident.ID
	pl.Tag = g.Param("tag")
	pl.You = getUser(g)

	if pl.Tag == "" {
		pl.Tag = "master"
		pl.IsGlobalTimeline = true
	} else {
		pl.IsTagTimeline = true
		if pl.You != nil {
			pl.ReplyView = makeReplyView(g, "")
			pl.IsTagTimelineFollowed = m.IsFollowing(pl.You.ID, "#"+pl.Tag)
		}
		a, _ := m.GetArticle(ident.NewID(ident.IDTagTag).SetTag(pl.Tag).String())
		if a != nil {
			pl.PostsUnderTag = int32(a.Replies)
		}
	}

	if g.Request.Method == "POST" {
		cbuf, _ := base64.StdEncoding.DecodeString(g.PostForm("cursors"))
		cursor = ident.UnmarshalID(cbuf)
	} else {
		if pl.IsGlobalTimeline {
			cursor = ident.NewID(ident.IDTagAuthor).SetTag(pl.Tag)
		} else {
			cursor = ident.NewID(ident.IDTagTag).SetTag(pl.Tag)
		}
	}

	a, next := m.WalkMulti(int(config.Cfg.PostsPerPage), cursor)
	fromMultiple(&pl.Articles, a, 0, getUser(g))

	for _, c := range next {
		pl.Next = base64.StdEncoding.EncodeToString(c.Marshal(nil))
		break
	}

	if g.PostForm("api") != "" {
		apiWrapper(g, pl.Next, pl.Articles)
		return
	}

	g.HTML(200, "timeline.html", pl)
}

func Timeline(g *gin.Context) {
	var pl ArticlesTimelineView
	pl.ReplyView = makeReplyView(g, "")
	pl.You = getUser(g)

	switch uid := g.Param("user"); {
	case uid != "" && uid != ":in":
		// View someone's timeline
		pl.IsUserTimeline = true
		pl.User, _ = m.GetUser(uid)
		if pl.User == nil {
			NotFound(g)
			return
		}
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
		pendingFCursor = next
	}

	if pl.IsUserTimeline || pl.IsInbox {
		if g.Request.Method == "POST" {
			cursors, _ = ident.SplitIDs(g.PostForm("cursors"))
		} else {
			if pl.IsUserTimeline {
				cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(pl.User.ID))
			} else {
				cursors = append(cursors, ident.NewID(ident.IDTagInbox).SetTag(pl.User.ID))
			}
		}
	} else if g.Request.Method == "POST" {
		var payload []byte
		cursors, payload = ident.SplitIDs(g.PostForm("cursors"))

		if len(payload) > 0 {
			readCursorsAndPendingFCursor(string(payload))
		}
	} else {
		pl.ShowNewPost = true
		readCursorsAndPendingFCursor("")
		cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(pl.User.ID))
	}

	a, next := m.WalkMulti(int(config.Cfg.PostsPerPage), cursors...)
	fromMultiple(&pl.Articles, a, 0, pl.You)

	if pl.IsInbox {
		go m.UpdateUser(pl.User.ID, func(u *mv.User) error {
			u.Unread = 0
			return nil
		})
	}

	pl.Next = ident.CombineIDs([]byte(pendingFCursor), next...)

	if g.PostForm("api") != "" {
		apiWrapper(g, pl.Next, pl.Articles)
		return
	}

	g.HTML(200, "timeline.html", pl)
}

func apiWrapper(g *gin.Context, next string, articles []ArticleView) {
	p := struct {
		EOT      bool
		Articles []string
		Next     string
	}{}

	p.Next = next
	p.EOT = p.Next == ""

	tmp := fakeResponseCatcher{}
	for _, a := range articles {
		tmp.Reset()
		engine.Engine.HTMLRender.Instance("row_content.html", a).Render(&tmp)
		p.Articles = append(p.Articles, tmp.String())
	}
	g.JSON(200, p)
}

func Replies(g *gin.Context) {
	var pl ArticleRepliesView
	var pid = g.Param("parent")

	parent, err := m.Get(pid)
	if err != nil || parent.ID == "" {
		NotFound(g)
		log.Println(pid, err)
		return
	}

	pl.ParentArticle.from(parent, _RichTime, getUser(g))
	pl.ReplyView = makeReplyView(g, pid)

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
	fromMultiple(&pl.Articles, a, _RichTime|_NoMoreParent|_ShowAvatar, getUser(g))
	pl.Next = next
	g.HTML(200, "post.html", pl)
}
