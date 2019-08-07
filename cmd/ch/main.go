package main

import (
	"crypto/aes"
	_ "image/png"
	"io/ioutil"
	"log"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/driver/chdropbox"
	"github.com/coyove/common/lru"
	"github.com/gin-gonic/gin"
	"github.com/globalsign/mgo/bson"
	"gopkg.in/yaml.v2"
)

func main() {
	m, err := NewManager("zzz", "mongodb://localhost")
	if err != nil {
		panic(err)
	}

	tmpa := []*Article{}
	tags := []string{"a", "B"}
	for i := 0; i < 100; i++ {
		a := NewArticle("hello"+strconv.Itoa(i), strconv.FormatUint(rand.Uint64(), 10)+"-"+strconv.Itoa(i), 100, nil, tags)
		tmpa = append(tmpa, a)
		if rand.Intn(2) == 0 && len(tmpa) > 0 {
			//m.PostReply(tmpa[rand.Intn(len(tmpa))].ID, a)
			m.PostReply(tmpa[0].ID, a)
		} else {
			m.PostArticle(a)
		}
	}

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

	var app = 20

	r := gin.Default()
	r.LoadHTMLGlob("template/*")
	r.Handle("POST", "/search", func(g *gin.Context) {
		g.Redirect(302, "/p/by_create/title:"+url.PathEscape(g.PostForm("q")))
	})
	r.Handle("GET", "/new/:id", func(g *gin.Context) {
		var pl = struct {
			UUID     string
			Reply    string
			Abstract string
		}{
			UUID: makeCSRFToken(g),
		}

		if id := g.Param("id"); id != "0" {
			pl.Reply = id
			a, err := m.GetArticle(displayIDToObejctID(id))
			if err != nil {
				log.Println(err)
				g.AbortWithStatus(400)
				return
			}
			if a.Title != "" {
				pl.Abstract = softTrunc(a.Title, 50)
			} else {
				pl.Abstract = softTrunc(a.Content, 50)
			}
		}
		g.HTML(200, "newpost.html", pl)
	})
	r.Handle("GET", "/edit/:id", func(g *gin.Context) {
		var pl = struct {
			UUID    string
			Reply   string
			Article *Article
		}{
			UUID:  makeCSRFToken(g),
			Reply: g.Param("id"),
		}

		a, err := m.GetArticle(displayIDToObejctID(pl.Reply))
		if err != nil {
			log.Println(err)
			g.AbortWithStatus(400)
			return
		}

		pl.Article = a
		g.HTML(200, "editpost.html", pl)
	})
	r.Handle("POST", "/edit", func(g *gin.Context) {
		if !isCSRFTokenValid(g, g.PostForm("uuid")) {
			g.AbortWithStatus(400)
			return
		}

		title := softTrunc(g.PostForm("title"), 64)
		content := softTrunc(g.PostForm("content"), int(config.MaxContent))
		author := authorNameToHash(g.PostForm("author"))
		tags := splitTags(g.PostForm("tags"))
		deleted := g.PostForm("delete")

		a, err := m.GetArticle(displayIDToObejctID(g.PostForm("reply")))
		if err != nil {
			g.AbortWithStatus(400)
			return
		}
		if a.Author != author && author != authorNameToHash(config.AdminName) {
			g.AbortWithStatus(400)
			return
		}
		if deleted == "" {
			if a.Parent == "" && len(title) < 10 {
				g.AbortWithStatus(400)
				return
			}
		}
		if err := m.UpdateArticle(a.ID, deleted != "", title, content, tags); err != nil {
			log.Println(err)
			g.AbortWithStatus(500)
			return
		}
		if a.Parent != "" {
			g.Redirect(302, "/p/by_create/parent:"+a.DisplayParentID())
		} else {
			g.Redirect(302, "/p/by_reply/index")
		}
	})
	r.Handle("POST", "/new", func(g *gin.Context) {
		if !isCSRFTokenValid(g, g.PostForm("uuid")) {
			g.AbortWithStatus(400)
			return
		}

		reply := displayIDToObejctID(g.PostForm("reply"))
		content := softTrunc(g.PostForm("content"), int(config.MaxContent))
		author := g.PostForm("author")
		if author == "" {
			if author = g.ClientIP(); author == "" {
				g.AbortWithStatus(400)
				return
			}
		}

		var err error
		if reply != "" {
			err = m.PostReply(reply, NewArticle("", content, authorNameToHash(author), nil, nil))
		} else {
			title := g.PostForm("title")
			if len(title) < 10 {
				title = "UNTITLED - " + time.Now().String()
			}
			title = softTrunc(title, 64)
			tags := splitTags(g.PostForm("tags"))
			err = m.PostArticle(NewArticle(title, content, authorNameToHash(author), nil, tags))
		}
		if err != nil {
			g.String(500, "Error: %v", err)
			return
		}
		if reply != "" {
			g.Redirect(302, "/p/by_create/parent:"+objectIDToDisplayID(reply)+"?p=-1")
		} else {
			g.Redirect(302, "/p/by_reply/index")
		}
	})
	r.Handle("GET", "/p/:sort/:type", func(g *gin.Context) {
		var (
			sort        = SortByReply
			findby      = ByNone()
			showReplies bson.ObjectId
			pl          struct {
				Articles      []*Article
				ParentArticle *Article
				Next, Prev    int64
				SortMode      string
				FindType      string
				SearchTerm    string
			}
		)

		if g.Param("sort") == "by_create" {
			sort = SortByCreate
		}

		if t := g.Param("type"); t == "index" {
			// ByNone
		} else if strings.HasPrefix(t, "tags:") {
			parts := strings.Split(t[5:], ",")
			findby = ByTags(parts...)
		} else if strings.HasPrefix(t, "title:") {
			pl.SearchTerm = t[6:]
			if strings.HasPrefix(pl.SearchTerm, "#") {
				findby = ByTags(splitTags(pl.SearchTerm)...)
			} else {
				findby = ByTitle(t[6:])
			}
		} else if strings.HasPrefix(t, "author:") {
			a, _ := strconv.ParseUint(t[7:], 36, 64)
			findby = ByAuthor(a)
		} else if strings.HasPrefix(t, "parent:") {
			pid := displayIDToObejctID(t[7:])
			if pid == "" {
				g.AbortWithStatus(404)
				return
			}
			findby = ByParent(pid)
			showReplies = pid
		} else {
			g.AbortWithStatus(404)
			return
		}

		var a []*Article
		next, err := strconv.Atoi(g.Query("n"))
		prev, err := strconv.Atoi(g.Query("p"))
		if prev != 0 {
			a, err = m.FindBack(findby, sort, int64(prev), app)
		} else {
			a, err = m.Find(findby, sort, int64(next), app)
		}

		if err != nil {
			g.AbortWithStatus(500)
			log.Println(err)
			return
		}

		pl.Articles = a
		pl.SortMode = string(sort)
		pl.FindType = g.Param("type")

		if showReplies != "" {
			pl.ParentArticle, err = m.GetArticle(showReplies)
			if err != nil {
				g.AbortWithStatus(404)
				log.Println(err)
				return
			}
		}

		if len(a) > 0 {
			if sort == SortByCreate {
				pl.Next = a[len(a)-1].CreateTime
				pl.Prev = a[0].CreateTime
			} else {
				pl.Next = a[len(a)-1].ReplyTime
				pl.Prev = a[0].ReplyTime
			}
		}

		g.HTML(200, "index.html", pl)
	})
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
