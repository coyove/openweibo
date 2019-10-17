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

func Home(g *gin.Context) {
	g.HTML(200, "home.html", struct {
		Tags []string
	}{
		config.Cfg.Tags,
	})
}

func Index(g *gin.Context) {
	var pl = ArticlesTimelineView{
		SearchTerm: g.Param("tag"),
		Tags:       config.Cfg.Tags,
	}

	opt := _obfs
	if manager.IsCrawler(g) {
		opt = 0
	}

	if strings.HasPrefix(pl.SearchTerm, "@@") {
		pl.SearchTerm = config.HashName(pl.SearchTerm[2:])
		opt |= _abstract
	} else if strings.HasPrefix(pl.SearchTerm, "@") {
		pl.SearchTerm = pl.SearchTerm[1:]
		opt |= _abstract
	} else if pl.SearchTerm != "" {
		pl.SearchTerm = "#" + pl.SearchTerm
	}

	cursor := ident.ParseDynamicID(g, g.Query("n")).String()
	a, next, err := m.Walk(pl.SearchTerm, cursor, int(config.Cfg.PostsPerPage))
	if err != nil {
		Error(500, "INTERNAL: "+err.Error(), g)
		return
	}

	fromMultiple(&pl.Articles, a, opt, g)

	pl.Next = next

	if u, _ := url.Parse(g.Request.Referer()); u != nil {
		pl.Prev = u.Query().Get("n")
		if pl.Prev <= pl.Next {
			pl.Prev = ""
		}
	}

	g.HTML(200, "index.html", pl)
}

func Replies(g *gin.Context) {
	ident.DecryptQuery(g)
	var pl = ArticleRepliesView{
		ShowIP: ident.IsAdmin(g),
		Tags:   config.Cfg.Tags,
	}
	var pid = g.Param("parent")
	var opt = _richtime | _showcontent

	if !manager.IsCrawler(g) {
		opt |= _obfs
	}

	if pl.ShowIP {
		ident.SetDecryptArticleIDCheckExp(false)
		defer ident.SetDecryptArticleIDCheckExp(true)
	}

	parent, err := m.Get(ident.ParseDynamicID(g, pid).String())
	if err != nil || parent.ID == "" {
		Error(404, "NOT FOUND", g)
		log.Println(pid, err)
		return
	}

	pl.ParentArticle.from(parent, opt, g)

	j := ident.ParseDynamicID(g, g.Query("j"))
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
		var answer [4]byte
		pl.ReplyView.UUID, answer = ident.MakeToken(g)
		pl.ReplyView.Challenge = ident.GenerateCaptcha(answer)
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

		fromMultiple(&pl.Articles, m.GetReplies(parent.ID, start+1, end+1), opt|_abstracttitle, g)

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
