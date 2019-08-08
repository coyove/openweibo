package main

import (
	"crypto/aes"
	"encoding/hex"
	_ "image/png"
	"io/ioutil"
	"log"
	"math/rand"
	"net/url"
	"strconv"
	"strings"

	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/driver/chdropbox"
	"github.com/coyove/common/lru"
	"github.com/dchest/captcha"
	"github.com/gin-gonic/gin"
	"github.com/globalsign/mgo/bson"
	"gopkg.in/yaml.v2"
)

var m *Manager

func main() {
	var err error
	m, err = NewManager("zzz", "mongodb://localhost")
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
	r.Use(mwIPThrot)
	r.LoadHTMLGlob("template/*")
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
	r.Handle("POST", "/search", func(g *gin.Context) {
		g.Redirect(302, "/p/by_create/title:"+url.PathEscape(g.PostForm("q")))
	})

	r.Handle("GET", "/new/:id", handleNewPostView)
	r.Handle("POST", "/new", handleNewPostAction)
	r.Handle("GET", "/edit/:id", handleEditPostView)
	r.Handle("POST", "/edit", handleEditPostAction)
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
