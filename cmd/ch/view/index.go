package view

import (
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/gin-gonic/gin"
)

var m *manager.Manager

type ArticlesTimelineView struct {
	Tags       []string
	Articles   []ArticleView
	Next       string
	Prev       string
	SearchTerm string
	Index      bool
}

type ArticleRepliesView struct {
	Tags          []string
	Articles      []ArticleView
	ParentArticle ArticleView
	CurPage       int
	TotalPages    int
	Pages         []int
	ShowIP        bool
	ReplyView     struct {
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
	var pl = ArticlesTimelineView{
		SearchTerm: g.Param("tag"),
		Tags:       config.Cfg.Tags,
	}
	var opt uint64

	if strings.HasPrefix(pl.SearchTerm, "@@") {
		if !g.GetBool("ip-ok") {
			Error(500, "NOT FOUND", g)
			return
		}
		pl.SearchTerm = config.HashName(pl.SearchTerm[2:])
		opt |= _abstract
	} else if strings.HasPrefix(pl.SearchTerm, "@") {
		pl.SearchTerm = pl.SearchTerm[1:]
		opt |= _abstract
	} else if pl.SearchTerm != "" {
		pl.SearchTerm = "#" + pl.SearchTerm
	}

	cursor := ident.ParseID(g.Query("n")).String()
	a, next, err := m.Walk(pl.SearchTerm, cursor, int(config.Cfg.PostsPerPage))
	if err != nil {
		Error(500, "INTERNAL: "+err.Error(), g)
		return
	}

	fromMultiple(&pl.Articles, a, opt)

	pl.Next = next
	pl.Index = cursor == ""

	if u, _ := url.Parse(g.Request.Referer()); u != nil {
		pl.Prev = u.Query().Get("n")
		if pl.Prev <= pl.Next || pl.Index {
			// If we are at the front page, or the prev page is smaller than the next page
			// then we consider the prev page invalid
			pl.Prev = ""
		}
	}

	g.HTML(200, "index.html", pl)
}

func Replies(g *gin.Context) {
	var pl = ArticleRepliesView{
		ShowIP: ident.IsAdmin(g),
		Tags:   config.Cfg.Tags,
	}
	var pid = g.Param("parent")
	var opt = _richtime | _showcontent

	parent, err := m.Get(ident.ParseID(pid).String())
	if err != nil || parent.ID == "" {
		Error(404, "NOT FOUND", g)
		log.Println(pid, err)
		return
	}

	pl.ParentArticle.from(parent, opt)
	pl.ParentArticle.Index = 0
	pl.ParentArticle.SubIndex = ""

	j := ident.ParseID(g.Query("j"))
	if idx := j.RIndex(); idx > 0 && int(idx) <= parent.Replies {
		p := intdivceil(int(idx), config.Cfg.PostsPerPage)
		g.Redirect(302, "/p/"+pid+"?p="+strconv.Itoa(p)+"#r"+strconv.Itoa(int(j.RIndex())))
		return
	}

	{
		pl.ReplyView.RContent = g.Query("content")
		pl.ReplyView.RAuthor = g.Query("author")
		pl.ReplyView.EError = g.Query("error")
		pl.ReplyView.ShowReply = g.Query("refresh") == "1" || pl.ReplyView.EError != ""
		if pl.ReplyView.RAuthor == "" {
			pl.ReplyView.RAuthor, _ = g.Cookie("id")
		}
		pl.ReplyView.UUID, pl.ReplyView.Challenge = ident.MakeToken(g)
	}

	pl.CurPage, _ = strconv.Atoi(g.Query("p"))
	pl.TotalPages = intdivceil(int(pl.ParentArticle.Replies), config.Cfg.PostsPerPage)

	//m.IncrCounter(g, parent.ID)

	if pl.CurPage == 0 {
		pl.CurPage = 1
	}
	pl.CurPage = intmin(pl.CurPage, pl.TotalPages)
	if pl.CurPage <= 0 {
		pl.CurPage = pl.TotalPages
	}

	if pl.TotalPages > 0 {
		start := intmin(int(pl.ParentArticle.Replies), (pl.CurPage-1)*config.Cfg.PostsPerPage)
		end := intmin(int(pl.ParentArticle.Replies), pl.CurPage*config.Cfg.PostsPerPage)

		fromMultiple(&pl.Articles, m.GetReplies(parent.ID, start+1, end+1), opt|_abstracttitle|_showsubreplies)

		// Fill in at most 7 page numbers for display
		pl.Pages = make([]int, 0, 8)
		for i := pl.CurPage - 3; i <= pl.CurPage+3 && i <= pl.TotalPages; i++ {
			if i > 0 {
				pl.Pages = append(pl.Pages, i)
			}
		}
		for last := pl.Pages[len(pl.Pages)-1]; len(pl.Pages) < 7 && last+1 <= pl.TotalPages; last = pl.Pages[len(pl.Pages)-1] {
			pl.Pages = append(pl.Pages, last+1)
		}
		for first := pl.Pages[0]; len(pl.Pages) < 7 && first-1 > 0; first = pl.Pages[0] {
			pl.Pages = append([]int{first - 1}, pl.Pages...)
		}
	}

	g.HTML(200, "post.html", pl)
}
