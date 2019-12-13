package view

import (
	"encoding/base64"
	"log"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/gin-gonic/gin"
)

var m *manager.Manager

type ArticlesTimelineView struct {
	Articles               []ArticleView
	Next                   string
	IsAdmin                bool
	IsInbox                bool
	IsUserTimeline         bool
	IsUserTimelineFollowed bool
	IsUserTimelineBlocked  bool
	IsGlobalTimeline       bool
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

	if g.Request.Method == "POST" {
		cbuf, _ := base64.StdEncoding.DecodeString(g.PostForm("cursors"))
		cursor = ident.UnmarshalID(cbuf)
	} else {
		cursor = ident.NewID(ident.IDTagAuthor).SetTag("master")
	}

	a, next := m.WalkMulti(int(config.Cfg.PostsPerPage), cursor)
	fromMultiple(&pl.Articles, a, 0)

	for _, c := range next {
		pl.Next = base64.StdEncoding.EncodeToString(c.Marshal(nil))
		break
	}

	pl.IsGlobalTimeline = true
	g.HTML(200, "timeline.html", pl)
}

func Timeline(g *gin.Context) {
	var pl ArticlesTimelineView
	pl.ReplyView = makeReplyView(g, "")

	u2, _ := g.Get("user")
	pl.You, _ = u2.(*mv.User)

	switch uid := g.Param("user"); {
	case uid != "" && uid != ":in":
		// View someone's timeline
		pl.IsUserTimeline = true
		pl.User, _ = m.GetUser(uid)
		if pl.User == nil {
			Error(404, "USER NOT FOUND", g)
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

	if pl.IsUserTimeline || pl.IsInbox {
		if g.Request.Method == "POST" {
			cbuf, _ := base64.StdEncoding.DecodeString(g.PostForm("cursors"))
			cursors = append(cursors, ident.UnmarshalID(cbuf))
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
			list, next := m.GetFollowingList(false, pl.User, string(payload), 1e6)
			for _, id := range list {
				if id.Followed {
					cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(id.ID))
				}
			}
			pendingFCursor = next
		}
	} else {
		pl.ShowNewPost = true

		list, next := m.GetFollowingList(false, pl.User, "", 1e6)
		for _, id := range list {
			if id.Followed {
				cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(id.ID))
			}
		}
		cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(pl.User.ID))
		pendingFCursor = next
	}

	a, next := m.WalkMulti(int(config.Cfg.PostsPerPage), cursors...)
	fromMultiple(&pl.Articles, a, 0)

	if pl.IsInbox {
		go m.UpdateUser(pl.User.ID, func(u *mv.User) error {
			u.Unread = 0
			return nil
		})
	}

	pl.Next = ident.CombineIDs([]byte(pendingFCursor), next...)
	g.HTML(200, "timeline.html", pl)
}

func Replies(g *gin.Context) {
	var pl ArticleRepliesView
	var pid = g.Param("parent")

	parent, err := m.Get(pid)
	if err != nil || parent.ID == "" {
		Error(404, "NOT FOUND", g)
		log.Println(pid, err)
		return
	}

	pl.ParentArticle.from(parent, _RichTime)
	pl.ReplyView = makeReplyView(g, pid)

	if u, ok := g.Get("user"); ok {
		pl.ReplyView.CanDelete = u.(*mv.User).ID == pl.ParentArticle.Author || u.(*mv.User).IsMod()
	}

	cursor := g.Query("n")
	if cursor == "" {
		cursor = parent.ReplyChain
	}

	a, next := m.WalkReply(int(config.Cfg.PostsPerPage), cursor)
	fromMultiple(&pl.Articles, a, _RichTime|_NoMoreParent|_ShowAvatar)
	pl.Next = next
	g.HTML(200, "post.html", pl)
}
