package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
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
	PostsUnderTag         int
	IsInbox               bool
	IsUserTimeline        bool
	IsUserLikeTimeline    bool
	IsTagTimelineFollowed bool
	IsTagTimeline         bool
	IsSearchTimeline      bool
	ShowNewPost           bool
	MediaOnly             bool
	User                  *model.User
	You                   *model.User
	Checkpoints           []string
	CurrentCheckpoint     string
	ReplyView             ReplyView
	HotTags               []HotTag
}

type ArticleRepliesView struct {
	Articles          []ArticleView
	ParentArticle     ArticleView
	Next              string
	ShowReplyLockInfo bool
	ReplyView         ReplyView
}

func Home(g *gin.Context) {
	if getUser(g) != nil {
		g.Redirect(302, "/t")
	} else {
		g.HTML(200, "home.html", nil)
	}
}

func S(g *gin.Context) {
	id := "S" + g.Param("id")
	if g.Query("raw") != "" {
		a, err := dal.GetArticle(id)
		if err != nil {
			log.Println(err)
			NotFound(g)
			return
		}
		g.String(200, a.Content)
		return
	}
	g.HTML(200, "S.html", struct {
		ID, ShortID string
	}{id, g.Query("short")})
}

func TagTimeline(g *gin.Context) {
	tag := g.Param("tag")
	pl := ArticlesTimelineView{
		Tag:           "#" + tag,
		You:           getUser(g),
		User:          &model.User{},
		IsTagTimeline: true,
		MediaOnly:     g.Query("media") != "",
	}

	// if pl.You == nil {
	// 	redirectVisitor(g)
	// 	return
	// }

	if pl.You != nil {
		pl.IsTagTimelineFollowed = dal.IsFollowing(pl.You.ID, pl.Tag)
	}

	a, _ := dal.GetArticle(ik.NewID(ik.IDTag, tag).String())
	if a != nil {
		pl.PostsUnderTag = a.Replies
		model.IndexTag(tag)
	}

	a2, next := dal.WalkMulti(pl.MediaOnly, int(common.Cfg.PostsPerPage), ik.NewID(ik.IDTag, tag))
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

	if pid := g.Query("pid"); pid != "" {
		g.Redirect(302, "/S/"+pid[1:])
		return
	}
	// if pl.You == nil {
	// 	redirectVisitor(g)
	// 	return
	// }

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
		pl.HotTags = TagHeat(g)
	case uid != "":
		// View someone's timeline
		pl.IsUserTimeline = true
		pl.Checkpoints = makeCheckpoints(g)
		pl.User, _ = dal.GetUserWithSettings(uid)
		if pl.User == nil {
			// 	if res := common.SearchUsers(uid, 1); len(res) == 1 {
			// 		pl.User, _ = dal.GetUser(res[0])
			// 		if pl.User != nil {
			// 			g.Redirect(302, "/t/"+url.PathEscape(pl.User.ID))
			// 			return
			// 		}
			// 	}
			NotFound(g)
			return
		}

		if pl.User.Settings().FollowerNeedsAcceptance != (time.Time{}) {
			if pl.You == nil {
				NotFound(g)
				return
			}
			if following, accepted := dal.IsFollowingWithAcceptance(pl.You.ID, pl.User); !following || !accepted {
				g.Set("need-accept", true)
				NotFound(g)
				return
			}
		}

		if pl.You != nil {
			pl.User.Buildup(pl.You)
		}
	default:
		// View my timeline
		if pl.You == nil {
			redirectVisitor(g)
			return
		}
		pl.User = pl.You
		pl.User.Buildup(pl.You)
	}

	cursors := []ik.ID{}
	pendingFCursor := ""

	if pl.User.ID == "master" {
		for i := 0; i < dal.Masters; i++ {
			master := "master"
			if i > 0 {
				master += strconv.Itoa(i)
			}
			cursors = append(cursors, ik.NewID(ik.IDAuthor, master))
		}
	} else if pl.IsUserTimeline {
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
	if pl.IsUserTimeline && pl.User.ID != "master" {
		// Remove anonymous articles from single UserTimeline because otherwise we are just idiots
		lastIsAnon := false // this ensure no one can post two adjacent anonymous articles
		for i := len(a) - 1; i >= 0; i-- {
			if a[i].Anonymous && !lastIsAnon {
				a = append(a[:i], a[i+1:]...)
				lastIsAnon = true
			} else {
				lastIsAnon = false
			}
		}
	}
	fromMultiple(&pl.Articles, a, 0, pl.You)

	pl.Next = ik.CombineIDs([]byte(pendingFCursor), next...)
	g.HTML(200, "timeline.html", pl)
}

func Inbox(g *gin.Context) {
	pl := ArticlesTimelineView{
		You:     getUser(g),
		User:    getUser(g),
		IsInbox: true,
	}

	if pl.You == nil {
		redirectVisitor(g)
		return
	}

	a, next := dal.WalkMulti(pl.MediaOnly, int(common.Cfg.PostsPerPage), ik.NewID(ik.IDInbox, pl.User.ID))
	fromMultiple(&pl.Articles, a, 0, pl.You)

	go dal.DoUpdateUser(&dal.UpdateUserRequest{
		ID:     pl.User.ID,
		Unread: aws.Int32(int32(0)),
	})

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
	if g.PostForm("search") == "true" {
		you := getUser(g)
		if you == nil {
			g.Status(403)
			return
		}

		start, _ := strconv.Atoi(g.PostForm("cursors"))
		a, next, _ := searchArticles(you, g.PostForm("searchtag"), start, nil)
		fromMultiple(&articles, a, 0, getUser(g))
		p.Next = next
	} else if g.PostForm("likes") == "true" {
		a, next := dal.WalkLikes(g.PostForm("media") == "true", int(common.Cfg.PostsPerPage), g.PostForm("cursors"))
		fromMultiple(&articles, a, 0, getUser(g))
		p.Next = next
	} else if g.PostForm("reply") == "true" {
		a, next := dal.WalkReply(int(common.Cfg.PostsPerPage), g.PostForm("cursors"))
		fromMultiple(&articles, a, _NoMoreParent|_ShowAuthorAvatar, getUser(g))
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

	you := getUser(g)
	// if you == nil {
	// 	g.Writer.Header().Add("X-Reason", "user/404")
	// 	g.Status(403)
	// 	return
	// }

	pl.ParentArticle.from(parent, _GreyOutReply, you)
	pl.ReplyView = makeReplyView(g, pid)

	if you != nil {
		if dal.IsBlocking(pl.ParentArticle.Author.ID, you.ID) {
			g.Status(404)
			return
		}

		pl.ShowReplyLockInfo = !(you.IsMod() || you.ID == pl.ParentArticle.Author.ID)
	}

	us := dal.GetUserSettings(pl.ParentArticle.Author.ID)
	if at := us.FollowerNeedsAcceptance; at != (time.Time{}) && !at.IsZero() {
		pl.ParentArticle.Author.SetSettings(us)
		if you == nil {
			g.Status(404)
			return
		}
		if _, accepted := dal.IsFollowingWithAcceptance(you.ID, pl.ParentArticle.Author); !accepted {
			g.Status(404)
			return
		}
	}

	a, next := dal.WalkReply(int(common.Cfg.PostsPerPage), parent.ReplyChain)
	fromMultiple(&pl.Articles, a, _NoMoreParent|_NoCluster|_ShowAuthorAvatar, getUser(g))
	pl.Next = next

	g.Writer.Header().Add("X-Reply", "true")
	g.HTML(200, "post.html", pl)
}

func Search(g *gin.Context) {
	pl := ArticlesTimelineView{
		You:              getUser(g),
		User:             getUser(g),
		Tag:              g.Param("query"),
		IsSearchTimeline: true,
	}

	if pl.You == nil {
		redirectVisitor(g)
		return
	}

	if pl.Tag == "" {
		g.HTML(200, "timeline.html", pl)
		return
	}

	as, next, err := searchArticles(pl.You, pl.Tag, 0, &pl.PostsUnderTag)
	if err != nil {
		g.Set("error", err.Error())
		NotFound(g)
		return
	}

	pl.Next = next
	fromMultiple(&pl.Articles, as, 0, pl.You)
	g.HTML(200, "timeline.html", pl)
}

func searchArticles(u *model.User, query string, start int, totalCount *int) ([]*model.Article, string, error) {
	timeout := time.Millisecond * 500
	if u.IsMod() {
		timeout = time.Second * 5
	}

	res, count, err := model.SearchArticle(query, timeout, start, common.Cfg.PostsPerPage+1)
	if err != nil {
		if err.Error() == context.DeadlineExceeded.Error() {
			return nil, "", fmt.Errorf("search_timeout")
		}
		return nil, "", fmt.Errorf("internal_error")
	}
	if totalCount != nil {
		*totalCount = (count)
	}

	as := []*model.Article{}
	for _, id := range res {
		if a, _ := dal.GetArticle(id); a != nil {
			if ik.ParseID(a.ID).Header() == ik.IDGeneral {
				as = append(as, a)
			}
		}
	}

	var next string
	if len(res) >= common.Cfg.PostsPerPage+1 {
		if len(as) >= common.Cfg.PostsPerPage+1 {
			as = as[:common.Cfg.PostsPerPage]
		}
		next = strconv.Itoa(start + common.Cfg.PostsPerPage)
	}

	return as, next, nil
}

func LocalImage(g *gin.Context) {
	img := g.Param("img")
	switch {
	case strings.HasPrefix(img, "/s/"):
		http.ServeFile(g.Writer, g.Request, "template/"+img[3:])
	case strings.HasPrefix(img, "/thumb/"):
		img = img[6:]
		fallthrough
	default:
		img = img[1:]
		cachepath := fmt.Sprintf("tmp/images/%s", img)
		http.ServeFile(g.Writer, g.Request, cachepath)
	}
}
