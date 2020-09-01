package main

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/dal/kv"
	"github.com/coyove/iis/dal/tagrank"
	"github.com/coyove/iis/handler"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/middleware"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/acme/autocert"
)

func main() {
	rand.Seed(time.Now().Unix())
	runtime.GOMAXPROCS(runtime.NumCPU())

	common.MustLoadConfig("config.json")
	common.LoadIPLocation()

	redisConfig := &kv.RedisConfig{
		Addr: common.Cfg.RedisAddr,
	}

	dal.Init(redisConfig, common.Cfg.DyRegion, common.Cfg.DyAccessKey, common.Cfg.DySecretKey)

	if common.Cfg.S3Region != "" {
		dal.S3 = kv.NewS3Storage(common.Cfg.S3Endpoint, common.Cfg.S3Region, common.Cfg.S3Bucket,
			common.Cfg.S3AccessKey, common.Cfg.S3SecretKey)
	}

	model.Init(redisConfig)
	model.OpenBleve("bleve.search")
	tagrank.Init(redisConfig)

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
		"sub": func(a, b int) int {
			return a - b
		},
		"ipChainLookup": func(chain string) [][3]interface{} {
			res := [][3]interface{}{}
			for _, part := range strings.Split(chain, ",") {
				part = strings.Trim(strings.TrimSpace(part), "{}")
				if len(part) == 0 {
					continue
				}
				var date time.Time
				data := strings.Split(part, "/")
				loc, _ := common.LookupIP(data[0])
				if len(data) > 1 {
					ts, _ := strconv.ParseInt(data[1], 36, 64)
					date = time.Unix(ts, 0)
				}
				res = append(res, [3]interface{}{data[0], loc, date})
			}
			return res
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
				return template.HTML("<span title='" + a.Format(time.ANSIC) + "' class='time day'>" + strconv.Itoa(int(s)/86400) + "</span>")
			}
			return template.HTML("<span title='" + a.Format(time.ANSIC) + "' class='time' data='" +
				strconv.FormatInt(a.Unix(), 10) + "'>" + a.Format("2006-01-02 <i>1504Z</i>") + "</span>")
		},
	})
	r.LoadHTMLGlob("template/*.html")
	r.Static("/s/", "template")

	r.NoRoute(handler.NotFound)
	r.Handle("GET", "/", handler.Home)
	r.Handle("GET", "/i/*img", handler.LocalImage)
	r.Handle("GET", "/avatar/:id", handler.Avatar)
	r.Handle("GET", "/tag/:tag", handler.TagTimeline)
	r.Handle("GET", "/user", handler.User)
	r.Handle("GET", "/user/:type", handler.UserList)
	r.Handle("GET", "/user/:type/:uid", handler.UserList)
	r.Handle("GET", "/likes/:uid", handler.UserLikes)
	r.Handle("GET", "/t", handler.Timeline)
	r.Handle("GET", "/t/:user", handler.Timeline)
	r.Handle("GET", "/search/:query", handler.Search)
	r.Handle("GET", "/search", handler.Search)
	r.Handle("GET", "/S/:id", handler.S)
	r.Handle("GET", "/inbox", handler.Inbox)
	r.Handle("GET", "/mod/user", handler.ModUser)
	r.Handle("GET", "/mod/kv", handler.ModKV)

	r.Handle("POST", "/api/upload_image", handler.APIUpload)
	r.Handle("POST", "/api/p/:parent", handler.APIReplies)
	r.Handle("POST", "/api/u/:id", handler.APIGetUserInfoBox)
	r.Handle("POST", "/api/timeline", handler.APITimeline)
	r.Handle("POST", "/api/user_kimochi", handler.APIUserKimochi)
	r.Handle("POST", "/api/new_captcha", handler.APINewCaptcha)
	r.Handle("POST", "/api/search", handler.APISearch)
	r.Handle("POST", "/api/ban", handler.APIBan)
	r.Handle("POST", "/api/promote_mod", handler.APIPromoteMod)
	r.Handle("POST", "/api/mod_kv", handler.APIModKV)
	r.Handle("POST", "/api/user_settings", handler.APIUpdateUserSettings)
	r.Handle("POST", "/api/clear_inbox", handler.APIClearInbox)
	r.Handle("POST", "/api2/follow_block", handler.APIFollowBlock)
	r.Handle("POST", "/api2/like_article", handler.APILike)
	r.Handle("POST", "/api2/signup", handler.APISignup)
	r.Handle("POST", "/api2/login", handler.APILogin)
	r.Handle("POST", "/api2/logout", handler.APILogout)
	r.Handle("POST", "/api2/new", handler.APINew)
	r.Handle("POST", "/api2/user_password", handler.APIUpdateUserPassword)
	r.Handle("POST", "/api2/delete", handler.APIDeleteArticle)
	r.Handle("POST", "/api2/toggle_nsfw", handler.APIToggleNSFWArticle)
	r.Handle("POST", "/api2/toggle_lock", handler.APIToggleLockArticle)
	r.Handle("POST", "/api2/drop_top", handler.APIDropTop)

	r.Handle("POST", "/rpc/user_info", handler.RPCGetUserInfo)

	r.Handle("GET", "/loaderio-4d068f605f9b693f6ca28a8ca23435c6", func(g *gin.Context) { g.String(200, ("loaderio-4d068f605f9b693f6ca28a8ca23435c6")) })

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

	if len(common.Cfg.Domains) > 0 {
		go func() {
			m := &autocert.Manager{
				Cache:      autocert.DirCache("secret-dir"),
				Prompt:     autocert.AcceptTOS,
				HostPolicy: autocert.HostWhitelist(common.Cfg.Domains...),
			}
			s := &http.Server{
				Addr:      ":https",
				TLSConfig: m.TLSConfig(),
				Handler:   r,
			}
			hello := &tls.ClientHelloInfo{ServerName: common.Cfg.Domains[0]}
			_, err := m.GetCertificate(hello)
			fmt.Println("ssl test:", err)
			fmt.Println(s.ListenAndServeTLS("", ""))
		}()
	}

	fmt.Println(r.Run(":5010"))
}
