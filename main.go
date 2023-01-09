package main

import (
	"flag"
	"net/http"
	"net/http/pprof"
	"os"

	"github.com/coyove/sdss/dal"
	"github.com/coyove/sdss/types"
	"github.com/sirupsen/logrus"
)

var debugRebuild = flag.Int("debug-rebuild", 0, "")

func main() {
	flag.Parse()

	types.LoadConfig("config.json")
	dal.InitDB()

	if *debugRebuild > 0 {
		rebuildData(*debugRebuild)
	}

	serve("/t/", HandleSingleTag)
	serve("/post", HandlePostPage)
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

	logrus.Info("start serving, pid=", os.Getpid())
	http.ListenAndServe(":8888", nil)
}

func generateSession(tag, name string, w http.ResponseWriter, r *http.Request) {
	req := types.Request{Request: r}
	_, v := req.GenerateSession(name)
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  v,
		Path:   "/",
		MaxAge: 365 * 86400,
	})
	logrus.Info("generate root session: ", v, " remote: ", req.RemoteIPv4())
	http.Redirect(w, r, "/", 302)
}
