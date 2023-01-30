package main

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/limiter"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/pierrec/lz4/v4"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

func downloadData() {
	downloadWiki := func(p string) ([]string, error) {
		req, _ := http.NewRequest("GET", "https://dumps.wikimedia.org/zhwiki/20230101/"+p, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		rd := bufio.NewReader(bzip2.NewReader(resp.Body))

		var res []string
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					return nil, err
				}
				break
			}
			parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
			x := parts[2]
			if strings.HasPrefix(x, "Category:") ||
				strings.HasPrefix(x, "WikiProject:") ||
				strings.HasPrefix(x, "Wikipedia:") ||
				strings.HasPrefix(x, "File:") ||
				strings.HasPrefix(x, "Template:") {
				continue
			}
			res = append(res, x)
		}
		return res, nil
	}

	for i, p := range strings.Split(`zhwiki-20230101-pages-articles-multistream-index1.txt-p1p187712.bz2
	zhwiki-20230101-pages-articles-multistream-index2.txt-p187713p630160.bz2
	zhwiki-20230101-pages-articles-multistream-index3.txt-p630161p1389648.bz2
	zhwiki-20230101-pages-articles-multistream-index4.txt-p1389649p2889648.bz2
	zhwiki-20230101-pages-articles-multistream-index4.txt-p2889649p3391029.bz2
	zhwiki-20230101-pages-articles-multistream-index5.txt-p3391030p4891029.bz2
	zhwiki-20230101-pages-articles-multistream-index5.txt-p4891030p5596379.bz2
	zhwiki-20230101-pages-articles-multistream-index6.txt-p5596380p7096379.bz2
	zhwiki-20230101-pages-articles-multistream-index6.txt-p7096380p8231694.bz2`, "\n") {
		v, err := downloadWiki(p)
		fmt.Println(p, len(v), err)

		buf := strings.Join(v, "\n")
		ioutil.WriteFile("data"+strconv.Itoa(i), []byte(buf), 0777)
	}

	f, _ := os.Open("out")
	rd := bufio.NewReader(f)
	data := map[string]bool{}
	for i := 0; ; i++ {
		line, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		if data[line] {
			continue
		}
		data[line] = true
	}
	f.Close()
	f, _ = os.Create("out2")
	for k := range data {
		f.WriteString(k)
	}
	f.Close()
}

func rebuildDataFromWiki(count int) {
	f, _ := os.Open("out.gz")
	gr, _ := gzip.NewReader(f)
	rd := bufio.NewReader(gr)

	lines := []string{}
	for i := 0; count <= 0 || i < count; i++ {
		line, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		line = html.UnescapeString(strings.TrimSpace(line))
		lines = append(lines, line)

		if len(lines) > 1000 {
			err := dal.Store.DB.Update(func(tx *bbolt.Tx) error {
				for _, line := range lines {
					i := clock.Id()
					k := types.Uint64Bytes(i)
					now := clock.UnixMilli() - 86400*1000*30
					ksv := dal.KeySortValue{
						Key:   k[:],
						Sort0: uint64(now),
						Sort1: []byte(line),
						Value: (&types.Note{
							Id:         uint64(i),
							Title:      line,
							Creator:    "bulk",
							CreateUnix: now,
							UpdateUnix: now,
						}).MarshalBinary(),
					}
					dal.KSVUpsert(tx, "notes", ksv)
					dal.KSVUpsert(tx, "creator_bulk", dal.KeySortValue{
						Key:   k[:],
						Sort0: uint64(now),
						Sort1: []byte(line),
					})
				}
				return nil
			})

			lines = lines[:0]
			log.Println(i, err)
		}
	}
}

func rebuildIndexFromDB() {
	dal.Store.Saver().Close()

	out := "data/rebuilt"
	os.RemoveAll(out)

	mgr, err := bitmap.NewManager(out, types.Config.Index.SwitchThreshold, types.Config.Index.CacheSize)
	if err != nil {
		logrus.Fatal("init bitmap manager: ", err)
	}

	dal.Store.DB.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte(dal.NoteBK + "_kv"))
		if bk == nil {
			return nil
		}

		c := bk.Cursor()
		i, tot := 0, bk.Sequence()
		for k, v := c.First(); len(k) > 0; k, v = c.Next() {
			note := types.UnmarshalNoteBinary(v)
			h := buildBitmapHashes(note.Title, note.Creator, note.ParentIds)
			mgr.Saver().AddAsync(bitmap.Uint64Key(note.Id), h)
			if i++; i%10000 == 0 {
				logrus.Infof("rebuild bitmap progress: %v / %v", i, tot)
			}
		}
		return nil
	})

	logrus.Infof("rebuild bitmap progress: done, wait for closing...")
	mgr.Saver().Close()

	logrus.Infof("remove current bitmaps: %v", os.RemoveAll("data/index"))
	logrus.Infof("rename rebuilt bitmaps: %v", os.Rename("data/rebuilt", "bitmap_cache/index"))
}

func compact(pCurrent, pTotal *int64) {
	oldPath := dal.Store.DB.Path()
	tmpPath := oldPath + ".compacted"
	os.Remove(tmpPath)

	oldfi, err := os.Stat(oldPath)
	if err != nil {
		logrus.Fatal("[compactor] sys stat: ", err)
	}
	*pTotal = oldfi.Size()

	db, err := bbolt.Open(tmpPath, 0777, &bbolt.Options{})
	if err != nil {
		logrus.Fatal("[compactor] ", err)
	}

	exited := false
	go func() {
		for i := 0; !exited; i++ {
			time.Sleep(time.Second)
			fi, err := os.Stat(tmpPath)
			if err != nil {
				if !exited {
					logrus.Error("[compactor] daemon sys stat: ", err)
				}
				break
			}
			*pCurrent = fi.Size()
			if i%10 == 0 {
				logrus.Infof("[compactor] progress: %d / %d (%d%%)",
					*pCurrent, *pTotal, int(float64(*pCurrent)/float64(*pTotal)*100))
			}
		}
	}()

	if err := bbolt.Compact(db, dal.Store.DB, 40960); err != nil {
		logrus.Fatal("[compactor] ", err)
	}

	db.Close()
	dal.Store.DB.Close()

	tmpfi, err := os.Stat(tmpPath)
	if err != nil {
		logrus.Fatal("[compactor] sys stat: ", err)
	}

	if err := os.Remove(oldPath); err != nil {
		logrus.Fatal("[compactor] remove old: ", err)
	}

	if err := os.Rename(tmpPath, oldPath); err != nil {
		logrus.Fatal("[compactor] rename: ", err)
	}

	logrus.Infof("[compactor] original size: %d", *pTotal)
	logrus.Infof("[compactor] compacted size: %d", tmpfi.Size())

	db, err = bbolt.Open(oldPath, 0777, dal.BBoltOptions)
	if err != nil {
		logrus.Fatal("[compactor] rename: ", err)
	}

	dal.Store.DB = db
	exited = true
}

func dump() error {
	oldPath := dal.Store.DB.Path()
	tmpPath := oldPath + ".dumped.lz4"
	os.Remove(tmpPath)

	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()

	out := lz4.NewWriter(f)
	return dal.Store.View(func(tx *bbolt.Tx) error {
		_, err := tx.WriteTo(out)
		return err
	})
}

func HandleRoot(w *types.Response, r *types.Request) {
	if ok, _ := limiter.CheckIP(r); !ok {
		w.WriteHeader(400)
		return
	}
	if rpwd := r.URL.Query().Get("rpwd"); rpwd == types.Config.RootPassword {
		_, v := r.GenerateSession('r')
		http.SetCookie(w, &http.Cookie{
			Name:   "session",
			Value:  v,
			Path:   "/",
			MaxAge: 365 * 86400,
		})
		logrus.Info("generate root session: ", v, " remote: ", r.RemoteIPv4)
		http.Redirect(w, r.Request, "/ns:root", 302)
		return
	} else if rpwd != "" {
		limiter.AddIP(r, 1)
		http.Redirect(w, r.Request, "/ns:root", 302)
		return
	}

	httpTemplates.ExecuteTemplate(w, "header.html", r)
	start := time.Now()
	fmt.Fprintf(w, "<title>ns:root</title>")
	if r.User.IsRoot() {
		fmt.Fprintf(w, "<pre class=wrapall>")
		fmt.Fprintf(w, "<a href='/ns:%v/debug/pprof/'>Go pprof</a>\n", rootUUID)
		fmt.Fprintf(w, "Dumper: /ns:%v/dump\n\n", rootUUID)
		fmt.Fprintf(w, "Your cookie: %s&emsp;", r.UserSession)
		fmt.Fprintf(w, "<button onclick=\"document.cookie='session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:01 GMT;';location.reload()\">Clear</button>\n\n")
		fmt.Fprintf(w, "Server started at %v\n", serverStart)

		fmt.Fprintf(w, "\n\n")

		df, _ := exec.Command("df", "-h").Output()
		fmt.Fprintf(w, "Disk:\n%s\n", df)

		uptime, _ := exec.Command("uptime").Output()
		fmt.Fprintf(w, "Uptime: %s\n", uptime)

		ic, _ := ioutil.ReadDir("image_cache")
		fmt.Fprintf(w, "Image cache: %d\n", len(ic))

		{
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			enc.Encode(dal.Store.Stats())
			w.Write([]byte("\n"))

			enc.Encode(dal.Metrics.Stats())
			w.Write([]byte("\n"))

			sz := dal.Store.Size()
			fmt.Fprintf(w, "Data on disk: %db (%.2fM)\n\n", sz, float64(sz)/1024/1024)
			sz2 := dal.Metrics.Size()
			fmt.Fprintf(w, "Metrics on disk: %db (%.2fM)\n\n", sz2, float64(sz2)/1024/1024)
		}

		dal.Store.WalkDesc(clock.UnixMilli(), func(b *bitmap.Range) bool {
			fmt.Fprint(w, b.String())
			return true
		})
		fmt.Fprintf(w, "\n\n(Rendered in %v)\n", time.Since(start))
		fmt.Fprintf(w, "</pre>")
	} else {
		fmt.Fprintf(w, `这里是后台登入页面，普通用户无需登入<br><br><form><input name=rpwd type=password> <input type=submit value="Root login"/></form>`)
	}
	httpTemplates.ExecuteTemplate(w, "footer.html", r)
}

func HandleMetrics(w *types.Response, r *types.Request) {
	nss := dal.MetricsListNamespaces()
	m := map[string][]dal.MetricsIndex{}
	now := clock.Unix() / 300
	start := now - 10*86400/300

	for _, ns := range nss {
		tmp := dal.MetricsRange(ns, start, now)
		if strings.HasPrefix(ns, "dbacc:") {
			tmp = dal.MetricsCalcAccDAvg(tmp)
		}
		m[ns] = tmp
	}

	r.AddTemplateValue("namespaces", nss)

	buf, _ := json.Marshal(m)
	r.AddTemplateValue("nss", nss)
	r.AddTemplateValue("metrics", *(*string)(unsafe.Pointer(&buf)))
	r.AddTemplateValue("start", start)
	r.AddTemplateValue("end", now)

	httpTemplates.ExecuteTemplate(w, "metrics.html", r)
}
