package main

import (
	"encoding/hex"
	"fmt"
	"html/template"
	_ "image/png"
	"io/ioutil"
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/coyove/ch/cache"
	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/driver/chdropbox"
	"github.com/dchest/captcha"
	"github.com/gin-gonic/autotls"
	"github.com/gin-gonic/gin"
)

var m *Manager

func main() {
	var err error
	m, err = NewManager("zzz", "mongodb://localhost")
	if err != nil {
		panic(err)
	}

	// titles := []string{
	// 	"ofo押金难退，你可以试试“假破产真逼债”，但是不建议 手机发帖  ...2 New	",
	// 	"各省适龄学生参加高考参加率以及其211、985录取率 手机发帖  ...2 New	",
	// 	"猫咪缺铁性贫血 该怎么补 attach_img New	",
	// 	"国产人造肉九月上市 手机发帖  ...2 New	",
	// 	"7天内美国三个枪手的照片排排看。 New	",
	// 	"台风让闻得出别人身上地铁站味道的小布尔乔亚崩溃了  ...23456 New	",
	// 	"为啥龙族吧会变成黑江南的呢？ 手机发帖  ...23	",
	// 	"“你可以不同意我的观点，但是我会捍卫你说话的权利”  ...23	",
	// 	"超员1人被查，司机怒将3岁儿子甩丢出去，准备驾车离开	",
	// 	"突然发现人与人的处境多么奇妙 attach_img	",
	// 	"秀得飞起的高速轮胎，开头无厘头片结尾惊悚片	",
	// 	"太极大师马保国亲戚张麒约战50岁业余摔跤手邓勇	",
	// 	"颜值成第一择偶条件 上海青年婚恋数据曝光  ...234	",
	// 	"咸阳51岁独居男家中去世 一周后被发现  ...2	",
	// 	"工商联：北京取消限购，各区单独设置新能源汽车号牌！	",
	// 	"乌贼娘《诡秘之主》集中讨论帖：scp克苏鲁蒸汽朋克 attach_img 手机发帖 heatlevel  ...23456..447	",
	// 	"脑洞，几个利奇马级别的台风能改变撒哈拉沙漠的地形地貌？	",
	// 	"华为这个次世代地图的饼画的还真有点意思 attach_img  ...23	",
	// 	"【持续更新图片】工作的营地隔壁发生了针对车辆IED炸弹... attach_img 手机发帖  ...234	",
	// 	"这不就是唐僧在车迟国比试的那个运动吗？ 手机发帖  ...2	",
	// 	"朋友家小区楼下的告示，管理人尽力了 手机发帖	",
	// 	"2.4米长眼镜王蛇爬入农家浴室 女主人被吓跑	",
	// 	"力保健也一般呐……	",
	// 	"那多笔记为什么感觉吊着一口气，欠点火候	",
	// 	"深圳1.5亿的房子长什么样子？! 手机发帖  ...2345	",
	// 	"想了解下论坛各位的父母吵架/相处情况 手机发帖  ...234	",
	// 	"求推万元左右的电钢琴 attach_img  ...2	",
	// 	"邮轮答疑帖S2 走咯？上船去咯？ attach_img	",
	// 	"总感觉有点怪，路上老有人找我问路 手机发帖  ...2	",
	// 	"微信表情包的麻将脸太大了，没了感觉 attach_img	",
	// 	"【树洞】爹妈年纪大了开始迷信，真是无解的难题 attach_img  ...2	",
	// 	"T恤设计将港澳归为国家 范思哲道歉：已下架并销毁 手机发帖	",
	// 	"洪阿姨勇气可嘉",
	// }

	loadConfig()
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

	mgr.LoadNodes(nodes)
	mgr.StartTransferAgent("tmp/transfer.db")
	cachemgr = cache.New("tmp/cache", config.CacheSize*1024*1024*1024, func(k string) ([]byte, error) {
		return mgr.Get(k)
	})
	go uploadLocalImages()

	r := gin.Default()

	if config.Key != "0123456789abcdef" {
		log.Println("P R O D U C A T I O N")
		gin.SetMode(gin.ReleaseMode)
		gin.DefaultWriter = ioutil.Discard
	}

	r.Use(mwRenderPerf, mwIPThrot)
	r.SetFuncMap(template.FuncMap{
		"RenderPerf": func() string {
			return fmt.Sprintf("%dms/%dms", survey.render.avg, survey.render.max)
		},
	})
	r.LoadHTMLGlob("template/*")
	r.Static("/s/", "static")
	r.Handle("GET", "/captcha/:challenge", func(g *gin.Context) {
		challenge, _ := hex.DecodeString(g.Param("challenge"))
		if len(challenge) != 16 {
			g.AbortWithStatus(400)
			return
		}
		config.blk.Decrypt(challenge, challenge)
		g.Writer.Header().Add("Content-Type", "image/png")
		// captcha package has been modified to suit my needs, so all future changes must happen in vendor folder
		captcha.NewImage(config.Key, challenge[:6], 180, 60).WriteTo(g.Writer)
	})
	r.Handle("GET", "/i/:image", handleImage)

	r.Handle("GET", "/", makeHandleMainView(0))
	r.Handle("GET", "/p/:parent", handleRepliesView)
	r.Handle("GET", "/tag/:tag", makeHandleMainView('t'))
	r.Handle("GET", "/search/:title", makeHandleMainView('T'))
	r.Handle("GET", "/tags", func(g *gin.Context) {
		g.HTML(200, "tags.html", struct{ Tags []string }{config.Tags})
	})
	r.Handle("GET", "/cookie", func(g *gin.Context) {
		id, _ := g.Cookie("id")
		g.HTML(200, "cookie.html", struct{ ID string }{id})
	})
	r.Handle("POST", "/cookie", func(g *gin.Context) {
		g.SetCookie("id", g.PostForm("id"), 86400*365, "", "", false, false)
		g.Redirect(302, "/")
	})
	r.Handle("POST", "/search", func(g *gin.Context) {
		g.Redirect(302, "/search/"+url.PathEscape(g.PostForm("q")))
	})
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
	r.Handle("GET", "/ip/:ip", makeHandleMainView('i'))

	r.Handle("GET", "/new/:id", handleNewPostView)
	r.Handle("POST", "/new", handleNewPostAction)
	r.Handle("GET", "/edit/:id", handleEditPostView)
	r.Handle("POST", "/edit", handleEditPostAction)
	r.Handle("GET", "/stat", handleCurrentStat)

	if config.Domain == "" {
		log.Fatal(r.Run(":5010"))
	} else {
		log.Fatal(autotls.Run(r, config.Domain))
	}
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
			pl.Title = "Tags: " + g.Param("tag")
		} else if t == 'T' {
			pl.SearchTerm = g.Param("title")
			if strings.HasPrefix(pl.SearchTerm, "#") {
				findby = ByTags(splitTags(pl.SearchTerm)...)
				pl.Title = "Tags: " + pl.SearchTerm
			} else {
				findby = ByTitle(expandText(pl.SearchTerm))
				pl.Title = "Search: " + pl.SearchTerm
			}
		} else if t == 'a' {
			name := strings.TrimRight(g.Param("id"), "*")
			a, _ := strconv.ParseUint(name, 36, 64)
			findby = ByAuthor(a)
			pl.Title = "Author: " + name
		} else if t == 'i' {
			ip, _ := strconv.ParseUint(g.Param("ip"), 10, 64)
			findby = ByIP(ip)
			pl.Title = "IP: " + (&Article{IP: ip}).IPString()
		} else {
			pl.Title = "Index"
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
			pl.Title += fmt.Sprintf(" = %s ~ %s", a[0].ReplyTimeString(), a[len(a)-1].ReplyTimeString())
		}

		g.HTML(200, "index.html", pl)
	}
}

func handleRepliesView(g *gin.Context) {
	var (
		pl   ArticlesView
		more bool
	)

	pl.ShowIP = isAdmin(g)
	pid := displayIDToObejctID(g.Param("parent"))
	if pid == "" {
		g.AbortWithStatus(404)
		return
	}

	next, err := strconv.Atoi(g.Query("n"))
	prev, err := strconv.Atoi(g.Query("p"))

	pl.ParentArticle, err = m.GetArticle(pid)
	if err != nil {
		g.AbortWithStatus(404)
		log.Println(err)
		return
	}

	if prev != 0 {
		pl.Articles, more, err = m.FindRepliesBack(pid, int64(prev), int(config.PostsPerPage))
		if !more {
			pl.NoPrev = true
		}
	} else {
		pl.Articles, more, err = m.FindReplies(pid, int64(next), int(config.PostsPerPage))
		if !more {
			pl.NoNext = true
		}
	}

	if err != nil {
		g.AbortWithStatus(500)
		log.Println(err)
		return
	}

	if len(pl.Articles) > 0 {
		pl.Next = pl.Articles[len(pl.Articles)-1].CreateTime
		pl.Prev = pl.Articles[0].CreateTime
	}

	g.HTML(200, "index.html", pl)
}
