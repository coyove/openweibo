package main

import (
	"crypto/aes"
	"encoding/hex"
	"fmt"
	_ "image/png"
	"io/ioutil"
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/driver/chdropbox"
	"github.com/coyove/common/lru"
	"github.com/dchest/captcha"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v2"
)

var m *Manager

func main() {
	var err error
	m, err = NewManager("zzz", "mongodb://localhost")
	if err != nil {
		panic(err)
	}

	//	tmpa := []*Article{}
	//	tags := []string{"a", "B"}
	//	for i := 0; i < 100; i++ {
	//		a := NewArticle("hello"+strconv.Itoa(i), strconv.FormatUint(rand.Uint64(), 10)+"-"+strconv.Itoa(i), 100, nil, tags)
	//		tmpa = append(tmpa, a)
	//		if rand.Intn(2) == 0 && len(tmpa) > 0 {
	//			//m.PostReply(tmpa[rand.Intn(len(tmpa))].ID, a)
	//			m.PostReply(tmpa[0].ID, a)
	//		} else {
	//			m.PostArticle(a)
	//		}
	//	}

	buf, err := ioutil.ReadFile("config.yml")
	if err != nil {
		panic(err)
	}

	if err := yaml.Unmarshal(buf, &config); err != nil {
		panic(err)
	}

	buf, _ = yaml.Marshal(config)
	log.Println("==== config:", string(buf))

	dedup = lru.NewCache(1024)
	config.Blk, _ = aes.NewCipher([]byte(config.Key))
	config.AdminNameHash = authorNameToHash(config.AdminName)

	nodes := []*driver.Node{}
	for _, s := range config.Storages {
		if s.Name == "" {
			panic("empty storage node name")
		}
		log.Println("[config] load storage:", s.Name)
		switch strings.ToLower(s.Type) {
		case "dropbox":
			nodes = append(nodes, chdropbox.NewNode(s.Name, s))
		default:
			panic("unknown storage type: " + s.Type)
		}
	}

	//mgr.LoadNodes(nodes)
	//mgr.StartTransferAgent("tmp/transfer.db")
	//cachemgr = cache.New("tmp/cache", config.CacheSize*1024*1024*1024, mgr.Get)
	//updateStat()

	r := gin.Default()
	r.Use(mwIPThrot)
	r.LoadHTMLGlob("template/*")
	r.Static("/s/", "static")
	r.Handle("GET", "/captcha/:challenge", func(g *gin.Context) {
		challenge, _ := hex.DecodeString(g.Param("challenge"))
		if len(challenge) != 16 {
			g.AbortWithStatus(400)
			return
		}
		config.Blk.Decrypt(challenge, challenge)
		g.Writer.Header().Add("Content-Type", "image/png")
		captcha.NewImage(config.Key, challenge[:6], 180, 60).WriteTo(g.Writer)
	})

	r.Handle("GET", "/", makeHandleMainView(0))
	r.Handle("GET", "/p/:parent", handleRepliesView)
	r.Handle("GET", "/tag/:tag", makeHandleMainView('t'))
	r.Handle("GET", "/search/:title", makeHandleMainView('T'))
	r.Handle("GET", "/tags", func(g *gin.Context) { g.HTML(200, "tags.html", struct{ Tags []string }{config.Tags}) })
	r.Handle("POST", "/search", func(g *gin.Context) { g.Redirect(302, "/search/"+url.PathEscape(g.PostForm("q"))) })
	r.Handle("GET", "/search", func(g *gin.Context) {
		if p := g.Query("provider"); p != "" {
			host := ""
			u, _ := url.Parse(g.Request.Referer())
			if u != nil {
				host = u.Host
			}
			g.Redirect(302, p+url.PathEscape("site:"+host+" "+g.Query("q")))
		} else {
			g.HTML(200, "search.html", nil)
		}
	})
	r.Handle("GET", "/id/:id", makeHandleMainView('a'))

	r.Handle("GET", "/new/:id", handleNewPostView)
	r.Handle("POST", "/new", handleNewPostAction)
	r.Handle("GET", "/edit/:id", handleEditPostView)
	r.Handle("POST", "/edit", handleEditPostAction)

	//r.Handle("GET", "/stat", func(g *gin.Context) {
	//	g.HTML(200, "stat.html", currentStat())
	//})
	//r.Handle("GET", "/i/:url", func(g *gin.Context) {
	//	url, k := g.Param("url"), ""
	//	if url == "raw" {
	//		url = g.Query("u")
	//		k = fmt.Sprintf("%x", sha1.Sum([]byte(url)))
	//	} else {
	//		k = strings.TrimRight(url, filepath.Ext(url))
	//	}
	//	buf, err := cachemgr.Fetch(k)
	//	if err != nil {
	//		g.String(500, "[ERR] fetch error: %v", err)
	//		return
	//	}
	//	g.Writer.Header().Add("Content-Type", "image/jpeg") // mime.TypeByExtension(filepath.Ext(url)))
	//	g.Writer.Write(buf)
	//})
	//r.Handle("POST", "/upload", func(g *gin.Context) {
	//	g.Request.Body = ioutil.NopCloser(io.LimitReader(g.Request.Body, 1024*1024))
	//	img, err := g.FormFile("image")
	//	if err != nil {
	//		g.String(400, "[ERR] bad request: %v", err)
	//		return
	//	}
	//	f, err := img.Open()
	//	if err != nil {
	//		g.String(500, "[ERR] upload: %v", err)
	//		return
	//	}
	//	buf, _ := ioutil.ReadAll(f)
	//	f.Close()

	//	k := fmt.Sprintf("%x", sha1.Sum([]byte(img.Filename)))
	//	if err := mgr.Put(k, buf); err != nil {
	//		g.String(500, "[ERR] put error: %v", err)
	//		return
	//	}

	//	if g.PostForm("web") == "1" {
	//		g.Redirect(302, fmt.Sprintf("/i/%s.jpg", k))
	//	} else {
	//		g.String(200, "[OK:%s.jpg] size: %.3fK, dim", k, float64(len(buf))/1024)
	//	}
	//})
	log.Fatal(r.Run(":5010"))
}

func makeHandleMainView(t byte) func(g *gin.Context) {
	return func(g *gin.Context) {
		var (
			findby = ByNone()
			pl     ArticlesView
		)

		if t == 't' {
			parts := strings.Split(g.Param("tag"), ",")
			findby = ByTags(parts...)
		} else if t == 'T' {
			pl.SearchTerm = g.Param("title")
			if strings.HasPrefix(pl.SearchTerm, "#") {
				findby = ByTags(splitTags(pl.SearchTerm)...)
			} else {
				findby = ByTitle(expandText(pl.SearchTerm))
			}
		} else if t == 'a' {
			a, _ := strconv.ParseUint(strings.TrimRight(g.Param("id"), "*"), 36, 64)
			findby = ByAuthor(a)
		}

		var a []*Article
		var more bool

		next, err := strconv.Atoi(g.Query("n"))
		prev, err := strconv.Atoi(g.Query("p"))

		if prev != 0 {
			a, more, err = m.FindBack(findby, int64(prev), int(config.PostsPerPage))
			if !more {
				pl.NoPrev = true
			}
		} else {
			a, more, err = m.Find(findby, int64(next), int(config.PostsPerPage))
			if !more {
				pl.NoNext = true
			}
		}

		if err != nil {
			g.AbortWithStatus(500)
			log.Println(err)
			return
		}

		pl.Articles = a
		if len(a) > 0 {
			pl.Next = a[len(a)-1].ReplyTime
			pl.Prev = a[0].ReplyTime
			pl.Title = fmt.Sprintf("%s - %s", a[0].ReplyTimeString(), a[len(a)-1].ReplyTimeString())
		}

		if pl.SearchTerm != "" {
			pl.Title = "search " + pl.SearchTerm
		}

		g.HTML(200, "index.html", pl)
	}
}

func handleRepliesView(g *gin.Context) {
	var (
		pl   ArticlesView
		a    []*Article
		more bool
	)

	pid := displayIDToObejctID(g.Param("parent"))
	if pid == "" {
		g.AbortWithStatus(404)
		return
	}

	next, err := strconv.Atoi(g.Query("n"))
	prev, err := strconv.Atoi(g.Query("p"))

	if prev != 0 {
		a, more, err = m.FindRepliesBack(pid, int64(prev), int(config.PostsPerPage))
		if !more {
			pl.NoPrev = true
		}
	} else {
		a, more, err = m.FindReplies(pid, int64(next), int(config.PostsPerPage))
		if !more {
			pl.NoNext = true
		}
	}

	if err != nil {
		g.AbortWithStatus(500)
		log.Println(err)
		return
	}

	pl.Articles = a
	pl.ParentArticle, err = m.GetArticle(pid)
	if err != nil {
		g.AbortWithStatus(404)
		log.Println(err)
		return
	}

	if len(a) > 0 {
		pl.Next = a[len(a)-1].CreateTime
		pl.Prev = a[0].CreateTime
	}

	g.HTML(200, "index.html", pl)
}
