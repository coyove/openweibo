package main

import (
	"io"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/common/sched"
	"github.com/coyove/iis/cmd/ch/id"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/autotls"
	"github.com/gin-gonic/gin"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
)

var m *Manager

func main() {
	rand.Seed(time.Now().Unix())
	runtime.GOMAXPROCS(4)
	sched.Verbose = false

	var err error
	loadConfig()
	loadTrafficCounter()

	m, err = NewManager("iis.db")
	if err != nil {
		panic(err)
	}

	os.MkdirAll("tmp/logs", 0700)
	logf, err := rotatelogs.New("tmp/logs/access_log.%Y%m%d%H%M", rotatelogs.WithLinkName("tmp/logs/access_log"), rotatelogs.WithMaxAge(7*24*time.Hour))
	logerrf, err := rotatelogs.New("tmp/logs/error_log.%Y%m%d%H%M", rotatelogs.WithLinkName("tmp/logs/error_log"), rotatelogs.WithMaxAge(7*24*time.Hour))
	if err != nil {
		panic(err)
	}

	if os.Getenv("BENCH") == "1" {
		ids := [][]byte{}
		randString := func() string { return strconv.Itoa(rand.Int())[:12] }
		names := []string{randString(), randString(), randString(), randString()}

		for i := 0; i < 1000; i++ {
			if rand.Intn(100) > 96 || len(ids) == 0 {
				a := m.NewPost("BENCH "+strconv.Itoa(i)+" post", strconv.Itoa(i), names[rand.Intn(len(names))], "127.0.0.0", "default")
				m.PostPost(a)
				ids = append(ids, a.ID)
			} else {
				a := m.NewReply("BENCH "+strconv.Itoa(i)+" reply", names[rand.Intn(len(names))], "127.0.0.0")
				if rand.Intn(4) == 1 {
					m.PostReply(ids[0], a)
				} else {
					m.PostReply(ids[rand.Intn(len(ids))], a)
				}
				ids = append(ids, a.ID)
			}

			if i%100 == 0 {
				log.Println("Progress", i)
			}
		}
	}

	if config.Key != "0123456789abcdef" {
		log.Println("P R O D U C A T I O N")
		gin.SetMode(gin.ReleaseMode)
		mwLoggerOutput, gin.DefaultErrorWriter = logf, logerrf
	} else {
		mwLoggerOutput, gin.DefaultErrorWriter = io.MultiWriter(logf, os.Stdout), io.MultiWriter(logerrf, os.Stdout)
	}

	log.SetOutput(mwLoggerOutput)
	log.SetFlags(log.Lshortfile | log.Ltime | log.Ldate)

	r := gin.New()
	r.Use(gin.Recovery(), gzip.Gzip(gzip.BestSpeed), mwLogger(), mwRenderPerf, mwIPThrot)
	r.NoRoute(func(g *gin.Context) { errorPage(404, "NOT FOUND", g) })
	r.LoadHTMLGlob("template/*.html")
	r.Static("/s/", "template")

	r.Handle("GET", "/", func(g *gin.Context) { g.HTML(200, "home.html", struct{ Tags []string }{config.Tags}) })
	r.Handle("GET", "/p/:parent", handleRepliesView)
	r.Handle("GET", "/cat", handleIndexView)
	r.Handle("GET", "/cat/:tag", handleIndexView)
	r.Handle("GET", "/new", handleNewPostView)
	r.Handle("GET", "/edit/:id", handleEditPostView)

	r.Handle("POST", "/new", handleNewPostAction)
	r.Handle("POST", "/reply", handleNewReplyAction)
	r.Handle("POST", "/edit", handleEditPostAction)
	r.Handle("POST", "/delete", handleDeletePostAction)

	r.Handle("GET", "/cookie", handleCookie)
	r.Handle("POST", "/cookie", handleCookie)

	r.Handle("GET", "/loaderio-4d068f605f9b693f6ca28a8ca23435c6", func(g *gin.Context) { g.String(200, ("loaderio-4d068f605f9b693f6ca28a8ca23435c6")) })

	if config.Domain == "" {
		log.Fatal(r.Run(":5010"))
	} else {
		go func() {
			time.Sleep(time.Second)
			log.Println("plain server started on :80")
			log.Fatal(r.Run(":80"))
		}()
		log.Fatal(autotls.Run(r, config.Domain))
	}
}

func handleIndexView(g *gin.Context) {
	var pl = ArticlesTimelineView{
		SearchTerm: g.Param("tag"),
		Tags:       config.Tags,
	}

	if strings.HasPrefix(pl.SearchTerm, "@") {
		pl.SearchTerm = pl.SearchTerm[1:]
		pl.ShowAbstract = true
	} else if pl.SearchTerm != "" {
		pl.SearchTerm = "#" + pl.SearchTerm
	}

	a, prev, next, err := m.FindPosts(pl.SearchTerm, id.StringBytes(g.Query("n")), int(config.PostsPerPage))
	if err != nil {
		errorPage(500, "INTERNAL: "+err.Error(), g)
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

func handleRepliesView(g *gin.Context) {
	var pl = ArticleRepliesView{
		ShowIP: isAdmin(g),
		Tags:   config.Tags,
	}
	var err error
	var pid = g.Param("parent")

	pl.ParentArticle, err = m.GetArticle(id.StringBytes(pid))
	if err != nil || pl.ParentArticle.ID == nil {
		errorPage(404, "NOT FOUND", g)
		log.Println(pl.ParentArticle, err)
		return
	}

	if idx := id.ParseID(g.Query("j")).RIndex(); idx > 0 && int64(idx) <= pl.ParentArticle.Replies {
		p := intdivceil(int(idx), config.PostsPerPage)
		g.Redirect(302, "/p/"+pid+"?p="+strconv.Itoa(p)+"#p"+g.Query("j"))
		return
	}

	pl.ReplyView = generateNewReplyView(pl.ParentArticle.ID, g)
	pl.CurPage, _ = strconv.Atoi(g.Query("p"))
	pl.TotalPages = intdivceil(int(pl.ParentArticle.Replies), config.PostsPerPage)

	incrCounter(g, pl.ParentArticle.ID)

	if pl.CurPage == 0 {
		pl.CurPage = 1
	}
	pl.CurPage = intmin(pl.CurPage, pl.TotalPages)
	if pl.CurPage < 0 {
		pl.CurPage = pl.TotalPages
	}

	if pl.TotalPages > 0 {
		start := intmin(int(pl.ParentArticle.Replies), (pl.CurPage-1)*config.PostsPerPage)
		end := intmin(int(pl.ParentArticle.Replies), pl.CurPage*config.PostsPerPage)

		pl.Articles = m.mgetReplies(pl.ParentArticle.ID, start+1, end+1)

		// Fill in at most 7 page numbers for display
		pl.Pages = make([]int, 0, 8)
		for i := pl.CurPage - 3; i <= pl.CurPage+3 && i <= pl.TotalPages && i > 0; i++ {
			pl.Pages = append(pl.Pages, i)
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
