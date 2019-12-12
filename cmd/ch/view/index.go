package view

import (
	"bytes"
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
	Articles       []ArticleView
	Next           string
	IsAdmin        bool
	IsInbox        bool
	IsUserTimeline bool
	Index          bool
	UUID           string
	User           *mv.User
}

type ArticleRepliesView struct {
	Articles        []ArticleView
	ParentArticle   ArticleView
	CanDeleteParent bool
	Next            string
	ReplyView       struct {
		UUID      string
		Challenge string
		ShowReply bool
		RAuthor   string
		RContent  string
		EError    string
	}
}

func SetManager(mgr *manager.Manager) {
	m = mgr
}

func Index(g *gin.Context) {
}

func Timeline(g *gin.Context) {
	var pl ArticlesTimelineView
	switch uid := g.Param("user"); {
	case uid != "" && uid != ":in":
		// View someone's timeline
		pl.IsUserTimeline = true
		pl.User, _ = m.GetUser(uid)
		if pl.User == nil {
			Error(404, "USER NOT FOUND", g)
			return
		}
	case uid == ":in":
		// View my inbox
		pl.IsInbox = true
		fallthrough
	default:
		// View my timeline
		u2, _ := g.Get("user")
		pl.User, _ = u2.(*mv.User)
		if pl.User == nil {
			g.Redirect(302, "/user")
			return
		}
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
		for cbuf, _ := base64.StdEncoding.DecodeString(g.PostForm("cursors")); len(cbuf) > 0; {
			if cbuf[0] == 0 {
				list, next := m.GetFollowingList(pl.User, string(cbuf[1:]), 1e6)
				for _, id := range list {
					cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(id.ID))
				}
				pendingFCursor = next
				break
			}

			id := ident.UnmarshalID(cbuf)
			if !id.Valid() {
				break
			}
			cbuf = cbuf[id.Size():]
			cursors = append(cursors, id)
		}
	} else {
		list, next := m.GetFollowingList(pl.User, "", 1e6)
		for _, id := range list {
			cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(id.ID))
		}
		cursors = append(cursors, ident.NewID(ident.IDTagAuthor).SetTag(pl.User.ID))
		pendingFCursor = next
	}

	a, next := m.WalkMulti(int(config.Cfg.PostsPerPage), cursors...)
	fromMultiple(&pl.Articles, a, 0)

	nextbuf := bytes.Buffer{}
	nextbuftmp := [32]byte{}
	for _, c := range next {
		p := c.Marshal(nextbuftmp[:])
		nextbuf.Write(p)
	}

	if pendingFCursor != "" {
		nextbuf.WriteByte(0)
		nextbuf.WriteString(pendingFCursor)
	}

	if pl.IsInbox {
		go m.UpdateUser(pl.User.ID, func(u *mv.User) error {
			u.Unread = 0
			return nil
		})
	}

	pl.Next = base64.StdEncoding.EncodeToString(nextbuf.Bytes())
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

	if u, ok := g.Get("user"); ok {
		pl.CanDeleteParent = u.(*mv.User).ID == pl.ParentArticle.Author || u.(*mv.User).IsMod()
	}

	pl.ReplyView.RContent = g.Query("content")
	pl.ReplyView.RAuthor = g.Query("author")
	pl.ReplyView.EError = g.Query("error")
	pl.ReplyView.ShowReply = g.Query("refresh") == "1" || pl.ReplyView.EError != ""
	if pl.ReplyView.RAuthor == "" {
		pl.ReplyView.RAuthor, _ = g.Cookie("id")
	}
	pl.ReplyView.UUID, pl.ReplyView.Challenge = ident.MakeToken(g)

	cursor := g.Query("n")
	if cursor == "" {
		cursor = parent.ReplyChain
	}

	a, next := m.WalkReply(int(config.Cfg.PostsPerPage), cursor)
	fromMultiple(&pl.Articles, a, _RichTime|_NoMoreParent|_ForceShowAvatar)
	pl.Next = next
	g.HTML(200, "post.html", pl)
}
