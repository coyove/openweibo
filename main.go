package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	listen          = flag.String("l", ":8888", "")
	rebuildFromWiki = flag.Int("rebuild-db", 0, "")
	rebuildIndex    = flag.Bool("rebuild-index", false, "")
	compactDB       = flag.Bool("compact", false, "")
	serverStart     time.Time
)

func main() {
	flag.Parse()
	rand.Seed(clock.Unix())
	serverStart = clock.Now()

	logrus.SetFormatter(&logFormatter{})
	logrus.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename:   "data/ns.log",
		MaxSize:    100,
		MaxBackups: 10,
		MaxAge:     7,
		Compress:   true,
	}))
	logrus.SetReportCaller(true)

	types.LoadConfig("config.json")
	dal.InitDB()

	// start := time.Now()
	// rand.Seed(start.UnixNano())
	// for i := 0; i < 1e5; i++ {
	// 	dal.MetricsUpdate(func(tx *bbolt.Tx) error {
	// 		bk, _ := tx.CreateBucketIfNotExists([]byte("test"))
	// 		bk.Put(types.Uint64Bytes(rand.Uint64()), nil)
	// 		return nil
	// 	})
	// }
	// fmt.Println(time.Since(start))
	// return

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

	serve("/", HandleView)
	serve("/ns:new", HandleNew)
	serve("/ns:edit", HandleEdit)
	serve("/ns:search", HandleTagSearch)
	serve("/ns:manage", HandleManage)
	serve("/ns:action", HandleAction)
	serve("/ns:history", HandleHistory)
	serve("/ns:root", HandleRoot)
	serve("/ns:metrics", HandleMetrics)

	http.Handle("/ns:"+rootUUID+"/debug/pprof/", http.StripPrefix("/ns:"+rootUUID, http.HandlerFunc(pprof.Index)))
	http.HandleFunc("/ns:"+rootUUID+"/dump", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		fmt.Fprintf(w, "%v in %v", dump(), time.Since(start))
	})
	http.HandleFunc("/ns:static/", HandleAssets)
	http.HandleFunc("/ns:image/", HandleImage)
	http.HandleFunc("/ns:thumb/", HandleImage)
	http.HandleFunc("/loaderio-9a0a63b98532fb7e58f548512801933c.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("loaderio-9a0a63b98532fb7e58f548512801933c"))
	})

	if types.Config.Domain != "" {
		autocertManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(types.Config.Domain),
			Cache:      autocert.DirCache("autocert_cache"),
		}
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/", autocertManager.HTTPHandler(nil))
			mux.HandleFunc("/ns:image/", HandleImage)
			mux.HandleFunc("/ns:thumb/", HandleImage)
			srv := &http.Server{
				Addr:         ":80",
				Handler:      mux,
				ReadTimeout:  time.Second,
				WriteTimeout: time.Second,
			}
			logrus.Fatal(srv.ListenAndServe())
		}()

		srv := &http.Server{
			Addr: ":443",
			TLSConfig: &tls.Config{
				GetCertificate:           autocertManager.GetCertificate,
				PreferServerCipherSuites: true,
				CurvePreferences:         []tls.CurveID{tls.X25519, tls.CurveP256},
			},
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      10 * time.Second,
		}

		logrus.Infof("start serving HTTPS + HTTP redirector, pid=%d, ServeUUID=%s", os.Getpid(), serveUUID)
		logrus.Fatal(srv.ListenAndServeTLS("", ""))
	} else {
		logrus.Infof("start serving HTTP %s, pid=%d, ServeUUID=%s", *listen, os.Getpid(), serveUUID)
		logrus.Fatal(http.ListenAndServe(*listen, nil))
	}
}

type logFormatter struct{}

func (f *logFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	buf := bytes.Buffer{}
	if entry.Level <= logrus.ErrorLevel {
		buf.WriteString("ERR")
	} else {
		buf.WriteString("INFO")
	}
	buf.WriteString("\t")
	buf.WriteString(entry.Time.UTC().Format("2006-01-02T15:04:05.000\t"))
	if entry.Caller == nil {
		buf.WriteString("internal")
	} else {
		buf.WriteString(filepath.Base(entry.Caller.File))
		buf.WriteString(":")
		buf.WriteString(strconv.Itoa(entry.Caller.Line))
	}
	buf.WriteString("\t")
	buf.WriteString(entry.Message)
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
