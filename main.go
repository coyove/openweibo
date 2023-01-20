package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	listen          = flag.String("l", ":8888", "")
	rebuildFromWiki = flag.Int("rebuild-db", 0, "")
	rebuildIndex    = flag.Bool("rebuild-index", false, "")
	compactDB       = flag.Bool("compact", false, "")
	reqMaxSize      = flag.Int64("request-max-size", 15, "")
	bitmapCacheSize = flag.Int64("bitmap-cache-size", 1024, "")
	serverStart     time.Time
)

func main() {
	flag.Parse()
	rand.Seed(clock.Unix())
	serverStart = clock.Now()

	logrus.SetFormatter(&LogFormatter{})
	logrus.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename:   "bitmap_cache/ns.log",
		MaxSize:    100,
		MaxBackups: 10,
		MaxAge:     7,
		Compress:   true,
	}))
	logrus.SetReportCaller(true)

	types.LoadConfig("config.json")
	dal.InitDB(*bitmapCacheSize * 1e6)

	if *compactDB {
		start := time.Now()

		logrus.Info("start serving maintenance page")

		var pc, pt int64
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "text/html")
			fmt.Fprintf(w, "<title>NoteServer</title><pre>[%v] Progress %02d%%, elapsed %.1fs, be patient...</pre>",
				time.Now().Format(time.Stamp),
				int(float64(pc)/float64(pt+1)*100),
				time.Since(start).Seconds())
		})
		srv := &http.Server{
			Addr:    *listen,
			Handler: mux,
		}
		go srv.ListenAndServe()

		compact(&pc, &pt)
		logrus.Infof("[compactor] elapsed: %v", time.Since(start))
		logrus.Infof("[compactor] shutdown: %v", srv.Shutdown(context.TODO()))
	}

	if *rebuildFromWiki > 0 {
		rebuildDataFromWiki(*rebuildFromWiki)
	}

	if *rebuildIndex {
		rebuildIndexFromDB()
		return
	}

	serve("/", HandleIndex)
	serve("/ns:new", HandleNew)
	serve("/ns:edit", HandleEdit)
	serve("/ns:search", HandleTagSearch)
	serve("/ns:manage", HandleManage)
	serve("/ns:action", HandleTagAction)
	serve("/ns:history", HandleHistory)
	serve("/ns:status", HandlePublicStatus)
	serve("/ns:notfound", func(w http.ResponseWriter, r *types.Request) {
		w.WriteHeader(404)
		httpTemplates.ExecuteTemplate(w, "404.html", r)
	})
	http.NotFoundHandler()

	root := types.UUIDStr()
	http.HandleFunc("/ns:"+root, func(w http.ResponseWriter, r *http.Request) {
		generateSession(root, "root", w, r)
	})
	http.HandleFunc("/ns:"+root+"/dump", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		fmt.Fprintf(w, "%v in %v", dump(), time.Since(start))
	})
	http.Handle("/ns:"+root+"/debug/pprof/", http.StripPrefix("/"+root, http.HandlerFunc(pprof.Index)))

	logrus.Info("root token page:  /ns:", root)
	logrus.Info("debug pprof page: /ns:", root, "/debug/pprof/")
	logrus.Info("dumper:           /ns:", root, "/dump")

	http.HandleFunc("/ns:static/", HandleAssets)
	http.HandleFunc("/ns:image/", HandleImage)
	http.HandleFunc("/ns:thumb/", HandleImage)

	logrus.Infof("start serving %s, pid=%d, ServeUUID=%s", *listen, os.Getpid(), serveUUID)

	// go func() {
	// 	time.Sleep(time.Second)
	// 	var wg sync.WaitGroup
	// 	start := time.Now()
	// 	for i := 0; i < 2000; i++ {
	// 		wg.Add(1)
	// 		go func() {
	// 			defer wg.Done()
	// 			http.Get("http://127.0.0.1:8888/ns:manage?desc=1&pid=Qawqwr71dxgSlZx_")
	// 		}()
	// 	}
	// 	wg.Wait()
	// 	fmt.Println(time.Since(start))
	// }()
	http.ListenAndServe(*listen, nil)
}

func generateSession(tag, name string, w http.ResponseWriter, r *http.Request) {
	req := types.Request{Request: r}
	_, v := req.GenerateSession('r')
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  v,
		Path:   "/",
		MaxAge: 365 * 86400,
	})
	logrus.Info("generate root session: ", v, " remote: ", req.RemoteIPv4)
	http.Redirect(w, r, "/", 302)
}
