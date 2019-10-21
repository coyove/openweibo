package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/coyove/common/sched"
	"github.com/coyove/iis/cmd/ch/action"
	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/engine"
	"github.com/coyove/iis/cmd/ch/manager"
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

	if os.Getenv("BENCH") == "1" {
		ids := []string{}
		randString := func() string { return strconv.Itoa(rand.Int())[:12] }
		names := []string{randString(), randString(), randString(), randString()}
		N := 40

		wg := sync.WaitGroup{}
		for i := 0; i < N; i++ {
			wg.Add(1)
			go func(i int) {
				a := m.NewPost("BENCH "+strconv.Itoa(i)+" post", strconv.Itoa(i), names[rand.Intn(len(names))], "127.0.0.0", "default")
				m.Post(a)
				ids = append(ids, a.ID)
				wg.Done()
			}(i)
		}
		wg.Wait()

		for k := 0; k < 2; k++ {
			wg.Add(1)
			go func() {
				time.Sleep(time.Second)
				x := append(names, "", "", "")
				m.Walk(x[rand.Intn(len(x))], "", rand.Intn(N/2)+N/2)
				wg.Done()
			}()

			for i := 0; i < 50; i++ {
				wg.Add(1)
				go func(i int) {
					a := m.NewReply("BENCH "+strconv.Itoa(i)+" reply", names[rand.Intn(len(names))], "127.0.0.0")
					if rand.Intn(4) == 1 {
						m.PostReply(ids[0], a)
					} else {
						m.PostReply(ids[rand.Intn(len(ids))], a)
					}
					ids = append(ids, a.ID)

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
	r.LoadHTMLGlob("template/*.html")
	r.Static("/s/", "template")

	r.NoRoute(view.NotFound)
	r.Handle("GET", "/", view.Home)
	r.Handle("GET", "/cat", view.Index)
	r.Handle("GET", "/cat/:tag", view.Index)
	r.Handle("GET", "/p/:parent", view.Replies)
	r.Handle("GET", "/new", view.New)
	r.Handle("GET", "/edit/:id", view.Edit)
	r.Handle("POST", "/new", action.New)
	r.Handle("POST", "/reply", action.Reply)
	r.Handle("POST", "/edit", action.Edit)
	r.Handle("POST", "/delete", action.Delete)
	r.Handle("POST", "/cookie", action.Cookie)

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
