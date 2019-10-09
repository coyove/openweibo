package view

import (
	"html/template"
	"log"
	"strconv"
	"strings"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	id "github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/gin-gonic/gin"
)

var m *manager.Manager

type ArticlesTimelineView struct {
	Tags         []string
	Articles     []*mv.Article
	Next         string
	Prev         string
	SearchTerm   string
	ShowAbstract bool
}

type ArticleRepliesView struct {
	Tags          []string
	Articles      []*mv.Article
	ParentArticle *mv.Article
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

	if strings.HasPrefix(pl.SearchTerm, "@") {
		pl.SearchTerm = pl.SearchTerm[1:]
		pl.ShowAbstract = true
	} else if pl.SearchTerm != "" {
		pl.SearchTerm = "#" + pl.SearchTerm
	}

	a, prev, next, err := m.Walk(pl.SearchTerm, id.StringBytes(g.Query("n")), int(config.Cfg.PostsPerPage))
	if err != nil {
		Error(500, "INTERNAL: "+err.Error(), g)
		return
	}

	pl.Articles = a

	for i, a := range pl.Articles {
		pl.Articles[i].BeReplied = a.Author != pl.SearchTerm
	}

	if len(a) > 0 {
		pl.Next, pl.Prev = id.BytesString(next), id.BytesString(prev)
	}

	g.HTML(200, "index.html", pl)
}

func Replies(g *gin.Context) {
	var pl = ArticleRepliesView{
		ShowIP: ident.IsAdmin(g),
		Tags:   config.Cfg.Tags,
	}
	var err error
	var pid = g.Param("parent")

	pl.ParentArticle, err = m.Get(id.StringBytes(pid))
	if err != nil || pl.ParentArticle.ID == nil {
		Error(404, "NOT FOUND", g)
		log.Println(pid, err)
		return
	}

	if idx := id.ParseID(g.Query("j")).RIndex(); idx > 0 && int64(idx) <= pl.ParentArticle.Replies {
		p := intdivceil(int(idx), config.Cfg.PostsPerPage)
		g.Redirect(302, "/p/"+pid+"?p="+strconv.Itoa(p)+"#p"+g.Query("j"))
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
		var answer [6]byte
		pl.ReplyView.UUID, answer = ident.MakeToken(g)
		pl.ReplyView.Challenge = ident.GenerateCaptcha(answer)
	}

	pl.CurPage, _ = strconv.Atoi(g.Query("p"))
	pl.TotalPages = intdivceil(int(pl.ParentArticle.Replies), config.Cfg.PostsPerPage)

	m.IncrCounter(g, pl.ParentArticle.ID)

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

		pl.Articles = m.GetReplies(pl.ParentArticle.ID, start+1, end+1)

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

func Cookie(g *gin.Context) {
	id, _ := g.Cookie("id")

	var p = struct {
		ID     string
		Config template.HTML
		Tags   []string
	}{
		id,
		template.HTML(config.Cfg.PublicString),
		config.Cfg.Tags,
	}

	if ident.IsAdmin(g) {
		p.Config = template.HTML(config.Cfg.PrivateString)
	}

	g.HTML(200, "cookie.html", p)
}
