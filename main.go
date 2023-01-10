package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var debugRebuild = flag.Int("debug-rebuild", 0, "")
var compactDB = flag.Bool("compact", false, "")
var listen = flag.String("l", ":8888", "")

func main() {
	flag.Parse()
	logrus.SetFormatter(&LogFormatter{})
	logrus.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename:   "bitmap_cache/ts.log",
		MaxSize:    100,
		MaxBackups: 10,
		MaxAge:     7,
		Compress:   true,
	}))
	logrus.SetReportCaller(true)

	types.LoadConfig("config.json")
	dal.InitDB()

	if *compactDB {
		start := time.Now()

		logrus.Info("start serving WIP page")

		var pc, pt int64
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "text/html")
			fmt.Fprintf(w, "<title>TagServer</title><pre>[%v] Progress %02d%%, elapsed %.1fs, be patient...</pre>",
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

	if *debugRebuild > 0 {
		rebuildData(*debugRebuild)
	}

	serve("/t/", HandleSingleTag)
	serve("/post", HandlePostPage)
	serve("/tag/store_status", HandleTagStoreStatus)
	serve("/tag/search", HandleTagSearch)
	serve("/tag/manage", HandleTagManage)
	serve("/tag/history", HandleTagHistory)
	serve("/tag/manage/action", HandleTagAction)

	root := types.UUIDStr()
	http.HandleFunc("/"+root, func(w http.ResponseWriter, r *http.Request) { generateSession(root, "root", w, r) })
	http.Handle("/"+root+"/debug/pprof/", http.StripPrefix("/"+root, http.HandlerFunc(pprof.Index)))

	logrus.Info("root token page:  /", root)
	logrus.Info("debug pprof page: /", root, "/debug/pprof/")

	http.Handle("/static/", http.StripPrefix("/", http.FileServer(http.FS(httpStaticAssets))))

	logrus.Infof("start serving %s, pid=%d", *listen, os.Getpid())
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
	logrus.Info("generate root session: ", v, " remote: ", req.RemoteIPv4())
	http.Redirect(w, r, "/", 302)
}
