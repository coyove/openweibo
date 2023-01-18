package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
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
		now := clock.Now()
		t := time.Unix(0, v*1e6)
		if now.YearDay() == t.YearDay() && now.Year() == t.Year() {
			return t.Format("15:04:05")
		}
		d := (now.Unix()-t.Unix())/86400 + 1
		return "<b>" + strconv.Itoa(int(d)) + "d.</b> " + t.Format("15:04:05")
	},
	"generatePages": func(p int, pages int) (a []int) {
		if pages > 0 {
			a = append(a, 1)
			i := p - 2
			if i < 2 {
				i = 2
			} else if i > 2 {
				a = append(a, 0)
			}
			for ; i < pages && len(a) < 6; i++ {
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
	"joinParentTitles": func(t *types.Note) (res string) {
		tmp := dal.LoadParentTitles(t.ParentIds)
		for i := range tmp {
			tmp[i] = types.SafeHTML(tmp[i])
		}
		return strings.Join(tmp, " ")
	},
	"add":        func(a, b int) int { return a + b },
	"uuid":       func() string { return types.UUIDStr() },
	"imageURL":   imageURL,
	"trunc":      types.UTF16Trunc,
	"renderClip": types.RenderClip,
	"safeHTML":   types.SafeHTML,
	"fullEscape": types.FullEscape,
}).ParseFS(httpStaticPages, "static/*.html"))

var serveUUID = types.UUIDStr()

var errorMessages = map[string]string{
	"INTERNAL_ERROR":     "服务器错误",
	"IP_BANNED":          "IP封禁",
	"MODS_REQUIRED":      "无管理员权限",
	"PENDING_REVIEW":     "修改审核中",
	"LOCKED":             "记事已锁定",
	"INVALID_CONTENT":    "无效内容，过长或过短",
	"EMPTY_TITLE":        "标题为空，请输入标题或者选择一篇父记事",
	"TITLE_TOO_LONG":     "标题过长",
	"CONTENT_TOO_LONG":   "内容过长",
	"TOO_MANY_PARENTS":   "父记事过多，最多8个",
	"DUPLICATED_TITLE":   "标题重名",
	"ILLEGAL_APPROVE":    "无权审核",
	"INVALID_ACTION":     "请求错误",
	"INVALID_IMAGE_NAME": "无效图片名",
	"INVALID_IMAGE":      "无效图片",
	"INVALID_PARENT":     "无效父记事",
	"CONTENT_TOO_LARGE":  "图片过大",
	"COOLDOWN":           "请稍后重试",
}

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
	fmt.Fprintf(w, "Your cookie: %s (Display: %s)\n\n", r.UserSession, r.UserDisplay)
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

		// time.Sleep(time.Second)
		f(w, req)
	}))
	http.Handle(pattern, h)
}

func writeJSON(w http.ResponseWriter, args ...interface{}) {
	m := map[string]interface{}{}
	for i := 0; i < len(args); i += 2 {
		k, v := args[i].(string), args[i+1]
		if k == "code" {
			m["msg"] = errorMessages[v.(string)]
		}
		m[k] = v
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
		ext := filepath.Ext(filepath.Base(hdr.Filename))
		if ext == "" {
			return ad, "INVALID_IMAGE_NAME"
		}

		seed := int(rand.Uint32() & 65535)
		ad.imageChanged = q.Get("image_changed") == "true"

		if q.Get("image_small") == "true" {
			ad.image, msg = saveImage(r, id, seed, ".s"+ext, img, hdr)
			if msg != "" {
				return ad, msg
			}
		} else {
			thumb, thhdr, _ := r.Request.FormFile("thumb")
			if thumb == nil || thhdr == nil {
				return ad, "INVALID_IMAGE"
			}
			ad.image, msg = saveImage(r, id, seed, ".f"+ext, img, hdr)
			if msg != "" {
				return ad, msg
			}
			_, msg = saveImage(r, id, seed, ".f.thumb.jpg", thumb, thhdr)
			if msg != "" {
				return ad, msg
			}
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
		res, _ := dal.BatchCheckNoteExistences(ad.parentIds)
		for _, ok := range res {
			if !ok {
				return ad, "INVALID_PARENT"
			}
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
	if uid != "" {
		tmp = append(tmp, ngram.StrHash(uid))
	}
	return tmp
}

func expandQuery(query string) (q string, parentIds []uint64, uid string) {
	for len(query) > 0 {
		idx := strings.IndexByte(query, ' ')
		for idx > 0 && (query)[idx-1] == '\\' {
			idx2 := strings.IndexByte((query)[idx+1:], ' ')
			if idx2 == -1 {
				idx = -1
			} else {
				idx = idx + 1 + idx2
			}
		}

		if strings.HasPrefix(query, "ns:id:") {
			if idx > 0 {
				id, _ := strconv.ParseUint((query)[6:idx], 10, 64)
				parentIds = append(parentIds, id)
				query = strings.TrimSpace((query)[idx+1:])
			} else {
				id, _ := strconv.ParseUint((query)[6:], 10, 64)
				parentIds = append(parentIds, id)
				query = ""
				break
			}
		} else if strings.HasPrefix(query, "ns:title:") {
			if idx > 0 {
				if note, _ := dal.GetNoteByName(types.UnescapeSpace((query)[9:idx])); note.Valid() {
					parentIds = append(parentIds, note.Id)
				}
				query = strings.TrimSpace((query)[idx+1:])
			} else {
				if note, _ := dal.GetNoteByName(types.UnescapeSpace((query)[9:])); note.Valid() {
					parentIds = append(parentIds, note.Id)
				}
				query = ""
				break
			}
		} else if strings.HasPrefix(query, "ns:user:") {
			if idx > 0 {
				uid = (query)[8:idx]
				query = strings.TrimSpace((query)[idx+1:])
			} else {
				uid = (query)[8:]
				query = ""
				break
			}
		} else {
			break
		}
	}
	q = query
	return
}

func collectSimple(q string, parentIds []uint64, uid string) ([]uint64, []bitmap.JoinMetrics) {
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
	var ids []uint64
	for _, kis := range res {
		ids = append(ids, kis.Key.LowUint64())
	}
	return ids, jms
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
