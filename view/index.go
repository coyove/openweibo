package view

import (
	"log"
	"net/url"
	"strings"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/middleware"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

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
	User                  *model.User
	You                   *model.User
	Checkpoints           []string
	CurrentCheckpoint     string
	ReplyView             ReplyView
}

type ArticleRepliesView struct {
	Articles      []ArticleView
	ParentArticle ArticleView
	Next          string
	ShowReplyBox  bool
	ReplyView     ReplyView
}

func S(g *gin.Context) {
	g.Redirect(302, "/t/master?pid=S"+g.Param("id"))
}

func Index(g *gin.Context) {
	pl := ArticlesTimelineView{
		Tag:           g.Param("tag"),
		You:           getUser(g),
		User:          &model.User{},
		IsTagTimeline: true,
		MediaOnly:     g.Query("media") != "",
		ReplyView:     makeReplyView(g, ""),
	}

	if pl.You != nil {
		pl.IsTagTimelineFollowed = dal.IsFollowing(pl.You.ID, "#"+pl.Tag)
	}

	a, _ := dal.GetArticle(ik.NewID(ik.IDTag, pl.Tag).String())
	if a != nil {
		pl.PostsUnderTag = int32(a.Replies)
	}

	a2, next := dal.WalkMulti(pl.MediaOnly, int(common.Cfg.PostsPerPage), ik.NewID(ik.IDTag, pl.Tag))
	fromMultiple(&pl.Articles, a2, 0, getUser(g))

	pl.Next = ik.CombineIDs(nil, next...)
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
		pl.User = &model.User{
			ID: "master",
		}
		pl.Checkpoints = makeCheckpoints(g)
		pl.IsUserTimeline = true
	case uid != "":
		// View someone's timeline
		pl.IsUserTimeline = true
		pl.Checkpoints = makeCheckpoints(g)
		pl.User, _ = dal.GetUserWithSettings(uid)
		if pl.User == nil {
			if res := common.SearchUsers(uid, 1); len(res) == 1 {
				pl.User, _ = dal.GetUser(res[0])
				if pl.User != nil {
					g.Redirect(302, "/t/"+url.PathEscape(pl.User.ID))
					return
				}
			}
			NotFound(g)
			return
		}

		if pl.You != nil && pl.You.ID != pl.User.ID {
			pl.User.SetIsFollowing(dal.IsFollowing(pl.You.ID, uid))
			pl.User.SetIsBlocking(dal.IsBlocking(pl.You.ID, uid))
			pl.User.SetIsNotYou(true)
		}
	default:
		// View my timeline
		if pl.You == nil {
			g.Redirect(302, "/user")
			return
		}
		pl.User = pl.You
	}

	cursors := []ik.ID{}
	pendingFCursor := ""

	if pl.IsUserTimeline {
		pl.CurrentCheckpoint = g.Query("cp")
		if a, _ := dal.GetArticle("u/"+pl.User.ID+"/checkpoint/"+pl.CurrentCheckpoint, true); a != nil {
			cursors = append(cursors, ik.ParseID(a.NextID))
		} else {
			// 2020-01 hack fix
			if pl.CurrentCheckpoint == "2020-01" {
				if a, _ := dal.GetArticle("u/"+pl.User.ID+"/checkpoint/0001-01", true); a != nil {
					cursors = append(cursors, ik.ParseID(a.NextID))
				}
			}
			cursors = append(cursors, ik.NewID(ik.IDAuthor, pl.User.ID))
		}
		cursors = cursors[:1]
	} else {
		pl.ShowNewPost = true
		list, next := dal.GetFollowingList(ik.NewID(ik.IDFollowing, pl.User.ID), "", 1e6, false)
		for _, id := range list {
			if id.Followed {
				if strings.HasPrefix(id.ID, "#") {
					cursors = append(cursors, ik.NewID(ik.IDTag, id.ID[1:]))
				} else {
					cursors = append(cursors, ik.NewID(ik.IDAuthor, id.ID))
				}
			}
		}
		pendingFCursor = next
		cursors = append(cursors, ik.NewID(ik.IDAuthor, pl.User.ID))
	}

	a, next := dal.WalkMulti(pl.MediaOnly, int(common.Cfg.PostsPerPage), cursors...)
	fromMultiple(&pl.Articles, a, 0, pl.You)

	pl.Next = ik.CombineIDs([]byte(pendingFCursor), next...)
	g.HTML(200, "timeline.html", pl)
}

func Inbox(g *gin.Context) {
	pl := ArticlesTimelineView{
		ReplyView: makeReplyView(g, ""),
		You:       getUser(g),
		User:      getUser(g),
		IsInbox:   true,
	}

	if pl.You == nil {
		g.Redirect(302, "/user")
		return
	}

	a, next := dal.WalkMulti(pl.MediaOnly, int(common.Cfg.PostsPerPage), ik.NewID(ik.IDInbox, pl.User.ID))
	fromMultiple(&pl.Articles, a, 0, pl.You)

	go dal.Do(dal.NewRequest("UpdateUser",
		"ID", pl.User.ID,
		"Unread", int32(0),
	))

	pl.Next = ik.CombineIDs(nil, next...)
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
		a, next := dal.WalkLikes(g.PostForm("media") == "true", int(common.Cfg.PostsPerPage), g.PostForm("cursors"))
		fromMultiple(&articles, a, 0, getUser(g))
		p.Next = next
	} else if g.PostForm("reply") == "true" {
		a, next := dal.WalkReply(int(common.Cfg.PostsPerPage), g.PostForm("cursors"))
		fromMultiple(&articles, a, _NoMoreParent|_ShowAvatar, getUser(g))
		p.Next = next
	} else {
		cursors, payload := ik.SplitIDs(g.PostForm("cursors"))

		var pendingFCursor string
		if len(payload) > 0 {
			list, next := dal.GetFollowingList(ik.ID{}, string(payload), 1e6, false)
			// log.Println(list, next, string(payload))
			for _, id := range list {
				if !id.Followed {
					continue
				}
				if strings.HasPrefix(id.ID, "#") {
					cursors = append(cursors, ik.NewID(ik.IDTag, id.ID[1:]))
				} else {
					cursors = append(cursors, ik.NewID(ik.IDAuthor, id.ID))
				}
			}
			pendingFCursor = next
		}

		a, next := dal.WalkMulti(g.PostForm("media") == "true", int(common.Cfg.PostsPerPage), cursors...)
		fromMultiple(&articles, a, 0, getUser(g))
		p.Next = ik.CombineIDs([]byte(pendingFCursor), next...)
	}

	p.EOT = p.Next == ""

	for _, a := range articles {
		p.Articles = append(p.Articles, [2]string{a.ID, middleware.RenderTemplateString("row_content.html", a)})
	}
	g.JSON(200, p)
}

func APIReplies(g *gin.Context) {
	var pl ArticleRepliesView
	var pid = g.Param("parent")

	parent, err := dal.GetArticle(pid)
	if err != nil || parent.ID == "" {
		g.Status(404)
		log.Println(pid, err)
		return
	}

	pl.ParentArticle.from(parent, 0, getUser(g))
	pl.ReplyView = makeReplyView(g, pid)

	if u := getUser(g); u != nil {
		if dal.IsBlocking(pl.ParentArticle.Author.ID, u.ID) {
			g.Status(404)
			return
		}
		pl.ShowReplyBox = u.IsMod() || u.ID == pl.ParentArticle.Author.ID || !pl.ParentArticle.Locked
	}

	a, next := dal.WalkReply(int(common.Cfg.PostsPerPage), parent.ReplyChain)
	fromMultiple(&pl.Articles, a, _NoMoreParent|_ShowAvatar, getUser(g))
	pl.Next = next

	g.Writer.Header().Add("X-Reply", "true")
	g.HTML(200, "post.html", pl)
}
