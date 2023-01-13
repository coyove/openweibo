package main

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/NYTimes/gziphandler"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/coyove/sdss/contrib/cursor"
	"github.com/coyove/sdss/contrib/ngram"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

func cleanTitle(v string) string {
	title := strings.TrimSpace(v)
	title = strings.Replace(title, "//", "/", -1)
	title = strings.TrimLeft(title, "/")
	title = strings.TrimRight(title, "/")
	return title
}

func serve(pattern string, f func(http.ResponseWriter, *types.Request)) {
	h := gziphandler.GzipHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := r.URL
		now := time.Now()
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("fatal serving %v: %v, trace: %s", url, r, debug.Stack())
			}
		}()

		req := &types.Request{
			Request:    r,
			Start:      now,
			Config:     dal.GetJsonizedNoteCache("ns:config"),
			RemoteIPv4: types.RemoteIPv4(r),
		}

		if s, created := req.ParseSession(); created {
			http.SetCookie(w, &http.Cookie{
				Name:   "session",
				Value:  s,
				Path:   "/",
				MaxAge: 365 * 86400,
			})
		}

		time.Sleep(time.Second)
		f(w, req)
	}))
	http.Handle(pattern, h)
}

func writeJSON(w http.ResponseWriter, args ...interface{}) {
	m := map[string]interface{}{}
	for i := 0; i < len(args); i += 2 {
		m[args[i].(string)] = args[i+1]
	}
	buf, _ := json.Marshal(m)
	w.Header().Add("Content-Type", "application/json")
	w.Write(buf)
}

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

func rebuildData(count int) {
	data := map[uint64]string{}
	mgr := dal.Store.Manager
	f, _ := os.Open("out.gz")
	gr, _ := gzip.NewReader(f)
	rd := bufio.NewReader(gr)
	for i := 0; count <= 0 || i < count; i++ {
		line, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		h := buildBitmapHashes(line)
		if len(h) == 0 {
			continue
		}
		id := clock.Id()
		data[id] = line
		k := bitmap.Uint64Key(id)
		mgr.Saver().AddAsync(k, h)
		if i%100000 == 0 {
			log.Println(i)
		}
	}
	mgr.Saver().Close()

	for len(data) > 0 {
		dal.Store.DB.Update(func(tx *bbolt.Tx) error {
			c := 0
			for i, line := range data {
				k := types.Uint64Bytes(uint64(i))
				now := clock.UnixMilli()
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

				delete(data, i)
				c++
				if c > 1000 {
					break
				}
			}
			return nil
		})
		fmt.Println(len(data))
	}
}

func buildBitmapHashes(line string) []uint64 {
	m := ngram.SplitMore(line)
	for k, v := range ngram.Split(line) {
		m[k] = v
	}
	return m.Hashes()
}

func collectSimple(q string) ([]bitmap.Key, []bitmap.JoinMetrics) {
	h := ngram.SplitMore(q).Hashes()

	var h2 []uint64
	var allLetter = true
	for _, r := range q {
		if unicode.IsLower(r) || unicode.IsUpper(r) {
		} else {
			allLetter = false
		}
	}
	if !allLetter {
		h2 = ngram.Split(q).Hashes()
	}

	if len(h) == 0 {
		return nil, nil
	}

	res, jms := dal.Store.CollectSimple(cursor.New(), bitmap.Values{Major: h2, Exact: h}, 2000)
	var tags []bitmap.Key
	for _, kis := range res {
		tags = append(tags, kis.Key)
	}
	return tags, jms
}

func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

//go:embed static/*
var httpStaticPages embed.FS

var httpTemplates = template.Must(template.New("ts").Funcs(template.FuncMap{
	"formatUnixMilli": func(v int64) string {
		return time.Unix(0, v*1e6).Format("2006-01-02 15:04:05")
	},
	"generatePages": func(p int, pages int) (a []int) {
		if pages > 0 {
			a = append(a, 1)
			i := p - 5
			if i < 2 {
				i = 2
			} else if i > 2 {
				a = append(a, 0)
			}
			for ; i < pages && len(a) < 10; i++ {
				a = append(a, i)
			}
			if a[len(a)-1] != pages {
				if a[len(a)-1]+1 != pages {
					a = append(a, 0)
				}
				a = append(a, pages)
			}
		}
		return
	},
	"add":  func(a, b int) int { return a + b },
	"uuid": func() string { return types.UUIDStr() },
}).ParseFS(httpStaticPages, "static/*.html"))
