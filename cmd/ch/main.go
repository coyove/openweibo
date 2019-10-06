package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"html/template"
	_ "image/png"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"strconv"
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

	if os.Getenv("IIS_NAME") != "" {
		s := make([]byte, 8)

		keybuf := []byte(config.Key)
		for i := 0; ; i++ {
			rand.Read(s)
			for i := range s {
				s[i] = "abcdefghijklmnopqrstuvwxyz0123456789"[s[i]%36]
			}

			h := hmac.New(sha1.New, keybuf)
			h.Write(s)
			h.Write(keybuf)

			x := h.Sum(nil)

			y := base64.URLEncoding.EncodeToString(x[:4])[:5]
			if (y[0] == 'y' || y[0] == 'Y') &&
				(y[1] == 'm' || y[1] == 'M') &&
				(y[2] == 'o' || y[2] == 'O' || y[2] == '0') &&
				(y[3] == 'u' || y[3] == 'U') &&
				(y[4] == 's' || y[4] == 'S' || y[4] == '5') {
				fmt.Println("\nresult:", string(s))
				break
			}

			if i%1e3 == 0 {
				fmt.Printf("\rprogress: %dk", i/1e3)
			}
		}
	}

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
				m.PostReply(ids[rand.Intn(len(ids))], a)
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
	r.SetFuncMap(template.FuncMap{})
	r.LoadHTMLGlob("template/*.html")
	r.Static("/s/", "template")
	r.Handle("GET", "/", func(g *gin.Context) { g.HTML(200, "home.html", struct{ Home template.HTML }{}) })
	r.Handle("GET", "/vec", makeHandleMainView('v'))
	r.Handle("GET", "/p/:parent", handleRepliesView)
	r.Handle("GET", "/cat/:tag", makeHandleMainView('t'))
	r.Handle("GET", "/cats", handleTags)
	r.Handle("GET", "/id/:id", makeHandleMainView('a'))
	r.Handle("GET", "/new", handleNewPostView)
	r.Handle("GET", "/stat", handleCurrentStat)
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

func handleCookie(g *gin.Context) {
	if g.Request.Method == "GET" {
		id, _ := g.Cookie("id")
		g.HTML(200, "cookie.html", struct{ ID string }{id})
		return
	}
	if id := g.PostForm("id"); g.PostForm("clear") != "" || id == "" {
		g.SetCookie("id", "", -1, "", "", false, false)
	} else if g.PostForm("notify") != "" {
		g.Redirect(302, "/inbox/"+id)
		return
	} else {
		g.SetCookie("id", id, 86400*365, "", "", false, false)
	}
	g.Redirect(302, "/vec")
}

func makeHandleMainView(t byte) func(g *gin.Context) {
	return func(g *gin.Context) {
		var bkName string
		var pl ArticlesTimelineView
		var err error

		switch t {
		case 't':
			pl.SearchTerm, pl.Type = g.Param("tag"), "tag"
			bkName = "#" + pl.SearchTerm
		case 'a':
			pl.SearchTerm, pl.Type = g.Param("id"), "id"
			bkName = pl.SearchTerm
		}

		var next = id.StringBytes(g.Query("n"))
		var prev []byte

		pl.Articles, prev, next, pl.TotalCount, err = m.FindPosts(bkName, next, int(config.PostsPerPage))
		pl.NoPrev = prev == nil

		if err != nil {
			errorPage(500, "INTERNAL: "+err.Error(), g)
			return
		}

		if t == 'a' {
			for i, a := range pl.Articles {
				pl.Articles[i].BeReplied = a.Author != pl.SearchTerm
			}
		}

		if len(pl.Articles) > 0 {
			pl.Next = id.BytesString(next)
			pl.Prev = id.BytesString(prev)
			pl.Title = fmt.Sprintf("%s ~ %s", pl.Articles[0].CreateTimeString(true), pl.Articles[len(pl.Articles)-1].CreateTimeString(true))
		}

		g.HTML(200, "index.html", pl)
	}
}

func handleRepliesView(g *gin.Context) {
	var pl = ArticleRepliesView{ShowIP: isAdmin(g)}
	var err error
	var pid = g.Param("parent")

	pl.ParentArticle, err = m.GetArticle(id.StringBytes(pid))
	if err != nil || pl.ParentArticle.ID == nil {
		errorPage(404, "NOT FOUND", g)
		log.Println(pl.ParentArticle, err)
		return
	}

	if idx := id.ParseID(g.Query("j")).RIndex(); idx > 0 && int64(idx) <= pl.ParentArticle.Replies {
		p := int(math.Ceil(float64(idx) / float64(config.PostsPerPage)))
		g.Redirect(302, "/p/"+pid+"?p="+strconv.Itoa(p)+"#p"+g.Query("j"))
		return
	}

	pl.ReplyView = generateNewReplyView(pl.ParentArticle.ID, g)
	pl.CurPage, _ = strconv.Atoi(g.Query("p"))
	pl.TotalPages = int(math.Ceil(float64(pl.ParentArticle.Replies) / float64(config.PostsPerPage)))

	incrCounter(g, pl.ParentArticle.ID)

	switch pl.CurPage {
	case 0:
		pl.CurPage = 1
	case -1:
		pl.CurPage = pl.TotalPages
	default:
		pl.CurPage = intmin(pl.CurPage, pl.TotalPages)
	}

	if pl.TotalPages > 0 {
		start := intmin(int(pl.ParentArticle.Replies), (pl.CurPage-1)*config.PostsPerPage)
		end := intmin(int(pl.ParentArticle.Replies), pl.CurPage*config.PostsPerPage)

		pl.Articles = mgetReplies(pl.ParentArticle.ID, start+1, end+1)
		pl.Pages = make([]int, 0, pl.TotalPages)

		for i := pl.CurPage - 3; i <= pl.CurPage+3; i++ {
			if i > 0 && i <= pl.TotalPages {
				pl.Pages = append(pl.Pages, i)
			}
		}

		for len(pl.Pages) < 7 {
			if last := pl.Pages[len(pl.Pages)-1]; last+1 <= pl.TotalPages {
				pl.Pages = append(pl.Pages, last+1)
			} else {
				break
			}
		}

		for len(pl.Pages) < 7 {
			if first := pl.Pages[0]; first-1 > 0 {
				pl.Pages = append([]int{first - 1}, pl.Pages...)
			} else {
				break
			}
		}
	}

	g.HTML(200, "post.html", pl)
}
