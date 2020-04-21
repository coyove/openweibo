package main

import (
	"html/template"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http/pprof"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/action"
	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/dal/forgettable/goforget"
	"github.com/coyove/iis/dal/kv/cache"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/middleware"
	"github.com/coyove/iis/model"
	"github.com/coyove/iis/view"
	"github.com/gin-gonic/gin"
)

func main() {
	rand.Seed(time.Now().Unix())
	runtime.GOMAXPROCS(runtime.NumCPU())

	common.MustLoadConfig()

	dal.Init(&cache.RedisConfig{
		Addr: common.Cfg.RedisAddr,
	}, common.Cfg.DyRegion, common.Cfg.DyAccessKey, common.Cfg.DySecretKey)

	if common.Cfg.RedisAddr != "" {
		goforget.Init(&cache.RedisConfig{Addr: common.Cfg.RedisAddr, BatchWorkers: 10})
	}

	if os.Getenv("BENCH") == "1" {
		// 	ids := []string{}
		// 	names := []string{"aa", "bb", "cc", "dd"}
		// 	N := 40

		// 	for i := 0; i < N; i++ {
		// 		time.Sleep(time.Second)
		// 		aid, _ := dal.Post(&model.Article{
		// 			Content: "BENCH " + strconv.Itoa(i) + " post",
		// 			IP:      "127.0.0.0",
		// 			NSFW:    true,
		// 		}, &model.User{
		// 			ID: names[rand.Intn(len(names))],
		// 		}, false)
		// 		ids = append(ids, aid.ID)
		// 	}

		// 	wg := sync.WaitGroup{}
		// 	for k := 0; k < 2; k++ {
		// 		wg.Add(1)
		// 		// go func() {
		// 		// 	time.Sleep(time.Second)
		// 		// 	x := append(names, "", "", "")
		// 		// 	m.Walk(ik.IDTagCategory, x[rand.Intn(len(x))], "", rand.Intn(N/2)+N/2)
		// 		// 	wg.Done()
		// 		// }()

		// 		for i := 0; i < 50; i++ {
		// 			wg.Add(1)
		// 			go func(i int) {
		// 				parent := ids[0]
		// 				if rand.Intn(4) == 1 {
		// 					parent = ids[rand.Intn(len(ids))]
		// 				}
		// 				aid, _ := dal.PostReply(parent, "BENCH "+strconv.Itoa(i)+" reply", "", &model.User{
		// 					ID: names[rand.Intn(len(names))],
		// 				}, "127.0.0.0", false, false)
		// 				ids = append(ids, aid.ID)

		// 				if i%10 == 0 {
		// 					log.Println("Progress", i)
		// 				}
		// 				wg.Done()
		// 			}(i)
		// 		}
		// 		wg.Wait()
		// 	}
	}

	prodMode := common.Cfg.Key != "0123456789abcdef"

	cssVersion := ".prod." + strconv.FormatInt(time.Now().Unix(), 10) + ".css"
	if !prodMode {
		cssVersion = ".test.css"
	}

	go func() {
		for {
			cssFiles, _ := ioutil.ReadDir("template/css")
			css := []string{}
			for _, f := range cssFiles {
				if path := "template/css/" + f.Name(); !strings.HasSuffix(f.Name(), ".tmpl.css") {
					os.Remove(path)
				} else {
					css = append(css, path)
				}
			}
			for _, path := range css {
				buf, _ := ioutil.ReadFile(path)
				common.CSSLightConfig.WriteTemplate(path+cssVersion, string(buf))
				common.CSSDarkConfig.WriteTemplate(path+".dark"+cssVersion, string(buf))
			}
			if prodMode {
				return
			}
			time.Sleep(time.Second)
		}
	}()

	r := middleware.New(prodMode)
	r.SetFuncMap(template.FuncMap{
		"cssVersion": func() string {
			return cssVersion
		},
		"emptyUser": func() model.User {
			u := model.Dummy
			return u
		},
		"blend": func(args ...interface{}) interface{} {
			return args
		},
		"getTotalPosts": func(id string) int {
			a, _ := dal.GetArticle(ik.NewID(ik.IDAuthor, id).String())
			if a != nil {
				return int(a.Replies)
			}
			return 0
		},
		"getLastActiveTime": func(id string) time.Time {
			return dal.LastActiveTime(id)
		},
		"formatTime": func(a time.Time) template.HTML {
			if a == (time.Time{}) || a.IsZero() || a.Unix() == 0 {
				return template.HTML("<span class='time none'></span>")
			}
			s := time.Since(a).Seconds()
			if s < 5 {
				return template.HTML("<span class='time now'></span>")
			}
			if s < 60 {
				return template.HTML("<span class='time sec'>" + strconv.Itoa(int(s)) + "</span>")
			}
			if s < 3600 {
				return template.HTML("<span class='time min'>" + strconv.Itoa(int(s)/60) + "</span>")
			}
			if s < 86400 {
				return template.HTML("<span class='time hour'>" + strconv.Itoa(int(s)/3600) + "</span>")
			}
			if s < 7*86400 {
				return template.HTML("<span class='time day'>" + strconv.Itoa(int(s)/86400) + "</span>")
			}
			return template.HTML("<span class='time' data='" + strconv.FormatInt(a.Unix(), 10) + "'>" + a.Format("2006-01-02") + "</span>")
		},
	})
	r.LoadHTMLGlob("template/*.html")
	r.Static("/s/", "template")

	r.NoRoute(view.NotFound)
	r.Handle("GET", "/", view.Home)
	r.Handle("GET", "/img/:img", view.Image)
	r.Handle("GET", "/i/:img", view.I)
	r.Handle("GET", "/eriri", view.RandomEririImage)
	r.Handle("GET", "/tag/:tag", view.Index)
	r.Handle("GET", "/user", view.User)
	r.Handle("GET", "/user/:type", view.UserList)
	r.Handle("GET", "/user/:type/:uid", view.UserList)
	r.Handle("GET", "/likes/:uid", view.UserLikes)
	r.Handle("GET", "/t", view.Timeline)
	r.Handle("GET", "/t/:user", view.Timeline)
	r.Handle("GET", "/S/:id", view.S)
	r.Handle("GET", "/inbox", view.Inbox)
	r.Handle("GET", "/avatar/:id", view.Avatar)
	r.Handle("GET", "/mod/user", view.ModUser)
	r.Handle("GET", "/mod/kv", view.ModKV)

	r.Handle("POST", "/api/p/:parent", view.APIReplies)
	r.Handle("POST", "/api/u/:id", view.APIGetUserInfoBox)
	r.Handle("POST", "/api/timeline", view.APITimeline)
	r.Handle("POST", "/api/user_kimochi", action.APIUserKimochi)
	r.Handle("POST", "/api/new_captcha", action.APINewCaptcha)
	r.Handle("POST", "/api/search", action.APISearch)
	r.Handle("POST", "/api/ban", action.APIBan)
	r.Handle("POST", "/api/promote_mod", action.APIPromoteMod)
	r.Handle("POST", "/api/mod_kv", action.APIModKV)
	r.Handle("POST", "/api/user_settings", action.APIUpdateUserSettings)
	r.Handle("POST", "/api/clear_inbox", action.APIClearInbox)
	r.Handle("POST", "/api2/follow_block", action.APIFollowBlock)
	r.Handle("POST", "/api2/like_article", action.APILike)
	r.Handle("POST", "/api2/signup", action.APISignup)
	r.Handle("POST", "/api2/login", action.APILogin)
	r.Handle("POST", "/api2/logout", action.APILogout)
	r.Handle("POST", "/api2/new", action.APINew)
	r.Handle("POST", "/api2/user_password", action.APIUpdateUserPassword)
	r.Handle("POST", "/api2/delete", action.APIDeleteArticle)
	r.Handle("POST", "/api2/toggle_nsfw", action.APIToggleNSFWArticle)
	r.Handle("POST", "/api2/toggle_lock", action.APIToggleLockArticle)
	r.Handle("POST", "/api2/drop_top", action.APIDropTop)

	r.Handle("POST", "/rpc/user_info", action.RPCGetUserInfo)

	r.Handle("GET", "/loaderio-4d068f605f9b693f6ca28a8ca23435c6", func(g *gin.Context) { g.String(200, ("loaderio-4d068f605f9b693f6ca28a8ca23435c6")) })

	// goforget.Incr("tagheat", "a", "b", "c")
	// goforget.Incr("tagheat", "a", "b", "d")
	// time.Sleep(time.Second)
	// goforget.Incr("tagheat", "b", "c", "d")
	// goforget.Incr("tagheat", "b")

	r.Handle("GET", "/debug/pprof/*name", func(g *gin.Context) {
		u, _ := g.Get("user")
		uu, _ := u.(*model.User)
		if uu == nil || !uu.IsAdmin() {
			g.Status(400)
			return
		}
		name := strings.TrimPrefix(g.Param("name"), "/")
		if name == "" {
			pprof.Index(g.Writer, g.Request)
		} else {
			pprof.Handler(name).ServeHTTP(g.Writer, g.Request)
		}
	})

	log.Fatal(r.Run(":5010"))
	// 	log.Fatal(autotls.Run(r, common.Cfg.Domain))
}
