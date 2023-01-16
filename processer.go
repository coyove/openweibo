package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/NYTimes/gziphandler"
	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/limiter"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/coyove/sdss/contrib/cursor"
	"github.com/coyove/sdss/contrib/ngram"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

//go:embed static/assets/*
var httpStaticAssets embed.FS

//go:embed static/*
var httpStaticPages embed.FS

var httpTemplates = template.Must(template.New("ts").Funcs(template.FuncMap{
	"formatUnixMilli": func(v int64) string {
		return time.Unix(0, v*1e6).Format("2006-01-02 15:04:05")
	},
	"formatUnixMilliBR": func(v int64) string {
		t := time.Unix(0, v*1e6).Format("15:04:05")
		d := (clock.UnixMilli() - v) / 1e3 / 86400
		if d < 1 {
			return t
		}
		return "<b>" + strconv.Itoa(int(d)) + "</b>d&emsp;" + t
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
	"add":      func(a, b int) int { return a + b },
	"uuid":     func() string { return types.UUIDStr() },
	"imageURL": imageURL,
	"imageBox": func(a string) string {
		if a == "" {
			return "" //"<div class='icon-doc-text-inv' style='margin-right: 0.5em'></div>"
		}
		return fmt.Sprintf("<div class='image-selector-container small'>"+
			"<a href='javascript:openImage(\"%s\")'><img class=image src='%s' data-src='%s'></a></div>",
			imageURL("image", a), imageURL("thumb", a), imageURL("image", a))
	},
	"renderClip": types.RenderClip,
	"safeHTML":   types.SafeHTML,
	"fullEscape": types.FullEscape,
}).ParseFS(httpStaticPages, "static/*.html"))

var serveUUID = types.UUIDStr()

func HandleIndex(w http.ResponseWriter, r *types.Request) {
	if r.URL.Path == "/" {
		HandleView("ns:welcome", w, r)
	} else {
		HandleView(strings.TrimPrefix(r.URL.Path, "/"), w, r)
	}
}

func HandleAssets(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/ns:")
	if strings.HasSuffix(p, ".ns") {
		p = p[:len(p)-3-32-1]
	}
	switch {
	case strings.HasSuffix(p, ".js"):
		w.Header().Add("Content-Type", "text/javascript")
	case strings.HasSuffix(p, ".css"):
		w.Header().Add("Content-Type", "text/css")
	}
	w.Header().Add("Cache-Control", "public, max-age=604800")

	buf, _ := httpStaticAssets.ReadFile(p)
	w.Write(buf)
}

func HandleImage(w http.ResponseWriter, r *http.Request) {
	var p string
	if strings.HasPrefix(r.URL.Path, "/ns:image/") {
		p = strings.TrimPrefix(r.URL.Path, "/ns:image/")
	} else {
		p = strings.TrimPrefix(r.URL.Path, "/ns:thumb/")
		ext := filepath.Ext(p)
		p = p[:len(p)-len(ext)] + ".thumb.jpg"
	}
	f, err := os.Open("image_cache/" + p)
	if err != nil {
		if os.IsNotExist(err) {
			w.WriteHeader(404)
		} else {
			logrus.Errorf("image server error %q: %v", p, err)
			w.WriteHeader(500)
		}
		return
	}
	defer f.Close()
	rd := bufio.NewReader(f)
	buf, _ := rd.Peek(512)
	w.Header().Add("Cache-Control", "public, max-age=8640000")
	w.Header().Add("Content-Type", http.DetectContentType(buf))
	io.Copy(w, rd)
}

func HandlePublicStatus(w http.ResponseWriter, r *types.Request) {
	fmt.Fprintf(w, "Server started at %v\n", serverStart)
	fmt.Fprintf(w, "Request max size: %v\n", *reqMaxSize)

	df, _ := exec.Command("df", "-h").Output()
	fmt.Fprintf(w, "Disk:\n%s\n", df)

	stats := dal.Store.DB.Stats()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(stats)
	w.Write([]byte("\n"))

	fi, err := os.Stat(dal.Store.DB.Path())
	if err != nil {
		fmt.Fprintf(w, "<failed to read data on disk>\n\n")
	} else {
		sz := fi.Size()
		fmt.Fprintf(w, "data on disk: %d (%.2f)\n\n", sz, float64(sz)/1024/1024)
	}

	dal.Store.WalkDesc(clock.UnixMilli(), func(b *bitmap.Range) bool {
		fmt.Fprint(w, b.String())
		return true
	})
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
			Request:     limiter.LimitRequestSize(r, *reqMaxSize),
			ServerStart: serverStart,
			Start:       now,
			Config:      dal.GetJsonizedNoteCache("ns:config"),
			RemoteIPv4:  types.RemoteIPv4(r),
			ServeUUID:   serveUUID,
		}

		if s, created := req.ParseSession(); created {
			http.SetCookie(w, &http.Cookie{
				Name:   "session",
				Value:  s,
				Path:   "/",
				MaxAge: 365 * 86400,
			})
		}

		// 	time.Sleep(time.Second)
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

type actionData struct {
	title, content, image string
	imageChanged          bool
	parentIds, hash       []uint64
}

func (ad actionData) String() string {
	return fmt.Sprintf("title: %d, content: %d, parents: %d, image: %q",
		len(ad.title), len(ad.content), len(ad.parentIds), ad.image)
}

func getActionData(id uint64, r *types.Request) (ad actionData, msg string) {
	q := r.Form

	img, hdr, _ := r.Request.FormFile("image")
	if img != nil {
		thumb, thhdr, _ := r.Request.FormFile("thumb")
		if thumb == nil || thhdr == nil {
			return ad, "INVALID_IMAGE"
		}
		ext := filepath.Ext(filepath.Base(hdr.Filename))
		if ext == "" {
			return ad, "INVALID_IMAGE_NAME"
		}

		seed := int(rand.Uint32() & 65535)
		ad.imageChanged = q.Get("image_changed") == "true"
		ad.image, msg = saveImage(r, id, seed, ext, img, hdr)
		if msg != "" {
			return ad, msg
		}
		_, msg = saveImage(r, id, seed, ".thumb.jpg", thumb, thhdr)
		if msg != "" {
			return ad, msg
		}
	}

	ad.title = types.CleanTitle(q.Get("title"))

	if strings.HasPrefix(ad.title, "ns:") && !r.User.IsRoot() {
		return ad, "MODS_REQUIRED"
	}

	if pt := q.Get("parents"); pt != "" {
		gjson.Parse(pt).ForEach(func(key, value gjson.Result) bool {
			ad.parentIds = append(ad.parentIds, key.Uint())
			return true
		})
		if len(ad.parentIds) > r.GetParentsMax() {
			return ad, "TOO_MANY_PARENTS"
		}
	}

	ad.hash = buildBitmapHashes(ad.title, "", ad.parentIds)
	if len(ad.hash) == 0 {
		return ad, "EMPTY_TITLE"
	}

	if types.UTF16LenExceeds(ad.title, r.GetTitleMaxLen()) {
		return ad, "TITLE_TOO_LONG"
	}

	ad.content = strings.TrimSpace(q.Get("content"))
	if types.UTF16LenExceeds(ad.content, r.GetContentMaxLen()) {
		return ad, "CONTENT_TOO_LONG"
	}
	return ad, ""
}

func buildBitmapHashes(line string, uid string, parentIds []uint64) []uint64 {
	m := ngram.SplitMore(line)
	for k, v := range ngram.Split(line) {
		m[k] = v
	}
	tmp := m.Hashes()
	for _, id := range parentIds {
		tmp = append(tmp, types.Uint64Hash(id))
	}
	if len(tmp) > 0 && uid != "" {
		tmp = append(tmp, ngram.StrHash(uid))
	}
	return tmp
}

func collectSimple(q string, parentIds []uint64, uid string) ([]bitmap.Key, []bitmap.JoinMetrics) {
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
	for _, id := range parentIds {
		h = append(h, types.Uint64Hash(id))
	}
	if uid != "" {
		h = append(h, ngram.StrHash(uid))
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

func deleteImage(id string) {
	if id == "" {
		return
	}
	os.Remove("image_cache/" + id)
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

func saveImage(r *types.Request, id uint64, seed int, ext string,
	img multipart.File, hdr *multipart.FileHeader) (string, string) {
	fn := fmt.Sprintf("%d/%x%04x-%x%s", types.Uint64Hash(id)%1024, clock.Unix(), seed, id, ext)
	path := "image_cache/" + fn
	os.MkdirAll(filepath.Dir(path), 0777)
	out, err := os.Create(path)
	if err != nil {
		logrus.Errorf("create image %s err: %v", path, err)
		return "", "INTERNAL_ERROR"
	}
	defer out.Close()
	if _, err := io.Copy(out, img); err != nil {
		logrus.Errorf("copy image to local %s err: %v", path, err)
		return "", "INTERNAL_ERROR"
	}
	return fn, ""
}

func imageURL(p, a string) string {
	if a == "" {
		return ""
	}
	return "/ns:" + p + "/" + a
}

func doubleCheckFilterResults(q string, pids []uint64, notes []*types.Note, f func(*types.Note) bool) {
	h := ngram.SplitMore(q)
	for _, note := range notes {
		if len(h) == 0 || ngram.SplitMore(note.Title).Contains(h) || note.ContainsParents(pids) {
			if !f(note) {
				break
			}
		}
	}
}

func sortNotes(notes []*types.Note, st int, desc bool) []*types.Note {
	switch st {
	case 1:
		if desc {
			sort.Slice(notes, func(i, j int) bool {
				if len(notes[i].Title) == len(notes[j].Title) {
					return notes[i].Title > notes[j].Title
				}
				return len(notes[i].Title) > len(notes[j].Title)
			})
		} else {
			sort.Slice(notes, func(i, j int) bool {
				if len(notes[i].Title) == len(notes[j].Title) {
					return notes[i].Title < notes[j].Title
				}
				return len(notes[i].Title) < len(notes[j].Title)
			})
		}
	default:
		if desc {
			sort.Slice(notes, func(i, j int) bool { return notes[i].UpdateUnix > notes[j].UpdateUnix })
		} else {
			sort.Slice(notes, func(i, j int) bool { return notes[i].UpdateUnix < notes[j].UpdateUnix })
		}
	}
	return notes
}
