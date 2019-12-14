package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coyove/common/sched"
	"github.com/coyove/iis/cmd/ch/action"
	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/engine"
	"github.com/coyove/iis/cmd/ch/manager"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/coyove/iis/cmd/ch/view"
	"github.com/gin-gonic/autotls"
	"github.com/gin-gonic/gin"
)

func main() {
	noHTTP := false
	flag.BoolVar(&noHTTP, "no-http", false, "")
	flag.Parse()

	rand.Seed(time.Now().Unix())
	runtime.GOMAXPROCS(runtime.NumCPU())

	sched.Verbose = false
	config.MustLoad()

	m := manager.New("iis.db")
	view.SetManager(m)
	action.SetManager(m)
	engine.SetManager(m)

	if os.Getenv("BENCH") == "1" {
		ids := []string{}
		names := []string{"aa", "bb", "cc", "dd"}
		N := 40

		for i := 0; i < N; i++ {
			time.Sleep(time.Second)
			aid, _ := m.Post("BENCH "+strconv.Itoa(i)+" post", "", &mv.User{
				ID: names[rand.Intn(len(names))],
			}, "127.0.0.0")
			ids = append(ids, aid)
		}

		wg := sync.WaitGroup{}
		for k := 0; k < 2; k++ {
			wg.Add(1)
			// go func() {
			// 	time.Sleep(time.Second)
			// 	x := append(names, "", "", "")
			// 	m.Walk(ident.IDTagCategory, x[rand.Intn(len(x))], "", rand.Intn(N/2)+N/2)
			// 	wg.Done()
			// }()

			for i := 0; i < 50; i++ {
				wg.Add(1)
				go func(i int) {
					parent := ids[0]
					if rand.Intn(4) == 1 {
						parent = ids[rand.Intn(len(ids))]
					}
					aid, _ := m.PostReply(parent, "BENCH "+strconv.Itoa(i)+" reply", "", &mv.User{
						ID: names[rand.Intn(len(names))],
					}, "127.0.0.0")
					ids = append(ids, aid)

					if i%10 == 0 {
						log.Println("Progress", i)
					}
					wg.Done()
				}(i)
			}
			wg.Wait()
		}
	}

	r := engine.New(config.Cfg.Key != "0123456789abcdef")
	r.SetFuncMap(template.FuncMap{
		"isCDError": func(s string) string {
			if strings.HasPrefix(s, "guard/cooling-down/") {
				return s[19 : len(s)-1]
			}
			return ""
		},
		"formatTime": func(a time.Time) template.HTML {
			s := time.Since(a).Seconds()
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
			return template.HTML("<span class='time'>" + a.Format("2006-01-02 15:04") + "</span>")
		},
	})
	r.LoadHTMLGlob("template/*.html")
	r.Static("/s/", "template")

	r.NoRoute(view.NotFound)
	r.Handle("GET", "/", view.Home)
	r.Handle("GET", "/img/:img", view.Image)
	r.Handle("GET", "/master", view.Index)
	r.Handle("POST", "/master", view.Index)
	r.Handle("GET", "/user", view.User)
	r.Handle("GET", "/user/:type", view.UserList)
	r.Handle("GET", "/t", view.Timeline)
	r.Handle("GET", "/t/:user", view.Timeline)
	r.Handle("POST", "/t", view.Timeline)
	r.Handle("POST", "/t/:user", view.Timeline)
	r.Handle("GET", "/p/:parent", view.Replies)
	r.Handle("GET", "/avatar/:id", view.Avatar)
	r.Handle("POST", "/user", action.User)
	r.Handle("POST", "/new", action.New)
	r.Handle("POST", "/reply", action.Reply)

	r.Handle("GET", "/loaderio-4d068f605f9b693f6ca28a8ca23435c6", func(g *gin.Context) { g.String(200, ("loaderio-4d068f605f9b693f6ca28a8ca23435c6")) })

	if config.Cfg.Domain == "" {
		log.Fatal(r.Run(":5010"))
	} else {
		if !noHTTP {
			go func() {
				time.Sleep(time.Second)
				fmt.Println("HTTP server started on :80")
				log.Fatal(r.Run(":80"))
			}()
		}
		fmt.Println("HTTPS server started on :443")
		log.Fatal(autotls.Run(r, config.Cfg.Domain))
	}
}
