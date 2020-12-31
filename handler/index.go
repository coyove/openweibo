package handler

import (
	"fmt"
	"net/http"
	"strconv"
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
	PostsUnderTag         int
	IsInbox               bool
	IsUserTimeline        bool
	IsUserLikeTimeline    bool
	IsTagTimelineFollowed bool
	IsTagTimeline         bool
	IsSearchTimeline      bool
	IsUserWaitAccept      bool
	ShowNewPost           bool
	MediaOnly             bool
	IsCrawler             bool
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

func Static(g *gin.Context, id, shortID string) {
	if g.Query("raw") != "" || strings.Contains(g.Request.UserAgent(), "curl") {
		a, err := dal.GetArticle(id)
		throw(err, "")
		g.String(200, a.Content)
		return
	}
	if common.IsCrawler(g) {
		pl, ok := makeReplies(g, id)
		if !ok {
			g.Status(404)
			return
		}
		g.HTML(200, "post.html", pl)
	} else {
		g.HTML(200, "S.html", struct{ ID string }{id})
	}
}

func S(g *gin.Context) {
	Static(g, "S"+g.Param("id"), g.Query("short"))
}

func TagTimeline(g *gin.Context) {
	tags := strings.Split(g.Param("tag"), " ")
	if max := 3; len(tags) > max {
		tags = tags[:max]
	}

	pl := ArticlesTimelineView{
		Tag:           "#" + strings.Join(tags, " #"),
		You:           getUser(g),
		User:          &model.User{},
		IsTagTimeline: true,
		IsCrawler:     common.IsCrawler(g),
		MediaOnly:     g.Query("media") != "",
	}

	// if pl.You == nil {
	// 	redirectVisitor(g)
	// 	return
	// }

	if pl.You != nil && len(tags) == 1 {
		pl.IsTagTimelineFollowed = dal.IsFollowing(pl.You.ID, pl.Tag)
	}

	var cursors []ik.ID
	for _, tag := range tags {
		a, _ := dal.GetArticle(ik.NewID(ik.IDTag, tag).String())
		if a != nil {
			pl.PostsUnderTag += a.Replies
			model.IndexTag(tag)
		}
		cursors = append(cursors, ik.NewID(ik.IDTag, tag))
	}

	a2, next := dal.WalkMulti(pl.MediaOnly, int(common.Cfg.PostsPerPage), cursors...)
	fromMultiple(g, &pl.Articles, a2, 0, getUser(g))

	pl.Next = ik.CombineIDs(nil, next...)
	g.HTML(200, "timeline.html", pl)
}

func Timeline(g *gin.Context) {
	pl := ArticlesTimelineView{
		ReplyView: makeReplyView(g, "", getUser(g)),
		You:       getUser(g),
		MediaOnly: g.Query("media") != "",
		IsCrawler: common.IsCrawler(g),
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
		pl.User, _ = dal.GetUser(uid)
		if pl.User == nil {
			NotFound(g)
			return
		}

		if !checkFollowApply(g, pl.User, pl.You) {
			return
		}

		if pl.You != nil {
			pl.User.Buildup(pl.You)
			if pl.You.ID != pl.User.ID {
				following, accpeted := dal.IsFollowingWithAcceptance(pl.User.ID, pl.You)
				pl.IsUserWaitAccept = following && !accpeted
			}
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
		cursors = append(cursors, ik.NewID(ik.IDAuthor, pl.User.ID))
		pl.CurrentCheckpoint = g.Query("cp")

		for cp, i := pl.CurrentCheckpoint, 0; cp != "" && i < 3; cp, i = lastMonth(cp), i+1 {
			if a, _ := dal.GetArticle("u/"+pl.User.ID+"/checkpoint/"+cp, true); a != nil {
				cursors = []ik.ID{ik.ParseID(a.NextID)}
				break
			}
		}
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
	fromMultiple(g, &pl.Articles, a, 0, pl.You)

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
	fromMultiple(g, &pl.Articles, a, 0, pl.You)

	go dal.DoUpdateUser(pl.User.ID, "Unread", int32(0))

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
		a, next := searchArticles(you, g.PostForm("searchtag"), start, nil)
		fromMultiple(g, &articles, a, 0, getUser(g))
		p.Next = next
	} else if g.PostForm("likes") == "true" {
		c := g.PostForm("cursors")
		you := getUser(g)
		if you == nil {
			g.Status(403)
			return
		}
		if x := ik.ParseID(c); x.Header() == ik.IDAuthor && you.ID != x.Tag() {
			if u, _ := dal.GetUser(x.Tag()); u != nil && u.HideLikes == 1 {
				g.Status(403)
				return
			}
		}
		a, next := dal.WalkLikes(g.PostForm("media") == "true", int(common.Cfg.PostsPerPage), c)
		fromMultiple(g, &articles, a, 0, getUser(g))
		p.Next = next
	} else if g.PostForm("reply") == "true" {
		a, next := dal.WalkReply(int(common.Cfg.PostsPerPage), g.PostForm("cursors"))
		fromMultiple(g, &articles, a, _NoMoreParent|_ShowAuthorAvatar, getUser(g))
		p.Next = next
	} else {
		getter := func(key string) string {
			v := g.PostForm(key)
			if v == "" {
				v = g.Query(key)
			}
			return v
		}
		cursors, payload := ik.SplitIDs(getter("cursors"))

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

		a, next := dal.WalkMulti(getter("media") == "true", int(common.Cfg.PostsPerPage), cursors...)
		fromMultiple(g, &articles, a, 0, getUser(g))
		p.Next = ik.CombineIDs([]byte(pendingFCursor), next...)
	}

	p.EOT = p.Next == ""

	if g.Request.Method == "POST" {
		for _, a := range articles {
			p.Articles = append(p.Articles, [2]string{a.ID, middleware.RenderTemplateString("row_content.html", a)})
		}
		g.JSON(200, p)
	} else {
		pl := ArticlesTimelineView{
			You:       getUser(g),
			User:      &model.Dummy,
			IsCrawler: common.IsCrawler(g),
			Next:      p.Next,
			Articles:  articles,
			MediaOnly: g.PostForm("media") == "true",
		}
		g.HTML(200, "timeline.html", pl)
	}
}

func APIReplies(g *gin.Context) {
	pl, ok := makeReplies(g, g.Param("parent"))
	if !ok {
		g.Status(404)
		return
	}
	g.Writer.Header().Add("X-Reply", "true")
	g.HTML(200, "post.html", pl)
}

func makeReplies(g *gin.Context, pid string) (pl ArticleRepliesView, ok bool) {
	parent, err := dal.GetArticle(pid)
	if err != nil || parent.ID == "" {
		return
	}

	you := getUser(g)
	pl.ParentArticle.from(parent, _GreyOutReply, you)
	pl.ReplyView = makeReplyView(g, pid, you)

	if you != nil {
		if dal.IsBlocking(pl.ParentArticle.Author.ID, you.ID) {
			return
		}

		pl.ShowReplyLockInfo = !(you.IsMod() || you.ID == pl.ParentArticle.Author.ID)
	}

	if pl.ParentArticle.Author.FollowApply != 0 {
		if you == nil {
			return
		}
		if _, accepted := dal.IsFollowingWithAcceptance(you.ID, pl.ParentArticle.Author); !accepted {
			return
		}
	}

	a, next := dal.WalkReply(int(common.Cfg.PostsPerPage), parent.ReplyChain)
	fromMultiple(g, &pl.Articles, a, _NoMoreParent|_NoCluster|_ShowAuthorAvatar, getUser(g))
	pl.Next = next
	return pl, true
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

	as, next := searchArticles(pl.You, pl.Tag, 0, &pl.PostsUnderTag)
	pl.Next = next
	fromMultiple(g, &pl.Articles, as, 0, pl.You)
	g.HTML(200, "timeline.html", pl)
}

func searchArticles(u *model.User, query string, start int, totalCount *int) ([]*model.Article, string) {
	// timeout := time.Millisecond * 500
	// if u.IsMod() {
	// 	timeout = time.Second * 5
	// }

	res, count := model.Search("art", query, start, common.Cfg.PostsPerPage+1)
	if totalCount != nil {
		*totalCount = (count)
	}

	as := []*model.Article{}
	for _, id := range res {
		if a, _ := dal.GetArticle(id.String()); a != nil {
			if ik.ParseID(a.ID).Header() == ik.IDGeneral && !a.IsDeleted() {
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

	return as, next
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
