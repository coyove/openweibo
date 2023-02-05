package main

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"net/http"
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
		return types.LocalTime(time.Unix(0, v*1e6)).Format("2006-01-02 15:04:05")
	},
	"formatUnixMilliBR": func(v int64) string {
		now := types.LocalTime(clock.Now())
		t := types.LocalTime(time.Unix(0, v*1e6))
		if now.Year() == t.Year() {
			if now.YearDay() == t.YearDay() {
				return t.Format("15:04:05")
			}
			return t.Format("01-02")
		}
		d := (now.Unix()-t.Unix())/86400 + 1
		return strconv.Itoa(int(d)) + "天前"
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
	"getParentsData": func(ids []uint64) string {
		buf := &bytes.Buffer{}
		notes, _ := dal.BatchGetNotes(ids)
		for i, n := range notes {
			if n.Title != "" {
				fmt.Fprintf(buf, " data%d='%s,%s'", i, n.IdStr(), html.EscapeString(n.Title))
			} else {
				fmt.Fprintf(buf, " data%d='%s,ns:id:%s'", i, n.IdStr(), n.IdStr())
			}
		}
		return buf.String()
	},
	"makeTitle": func(t *types.Note, max int, hl bool) string {
		tt := types.SafeHTML(t.Title)
		if tt == "" {
			notes, _ := dal.BatchGetNotes(t.ParentIds)
			if len(notes) > 0 {
				sort.Slice(notes, func(i, j int) bool { return notes[i].ChildrenCount > notes[j].ChildrenCount })
				buf := &bytes.Buffer{}
				for _, n := range notes {
					if hl {
						buf.WriteString("<b>#</b>")
					} else {
						buf.WriteString("#")
					}
					buf.WriteString(n.HTMLTitleDisplay())
					buf.WriteString(" ")
					if buf.Len() > max {
						break
					}
				}
				return buf.String()
			}
			return t.HTMLTitleDisplay()
		}
		return tt
	},
	"add":        func(a, b int) int { return a + b },
	"mul":        func(a, b int) int { return a * b },
	"uuid":       func() string { return types.UUIDStr() },
	"imageURL":   imageURL,
	"equ64s":     types.EqualUint64,
	"trunc":      types.UTF16Trunc,
	"renderClip": types.RenderClip,
	"safeHTML":   types.SafeHTML,
	"fullEscape": types.FullEscape,
}).ParseFS(httpStaticPages, "static/*.html"))

var serveUUID = types.UUIDStr()

var rootUUID = types.UUIDStr()

var servedPaths = map[string]bool{}

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
	case strings.HasSuffix(p, ".png"):
		w.Header().Add("Content-Type", "image/png")
	}
	w.Header().Add("Cache-Control", "public, max-age=8640000")

	buf, _ := httpStaticAssets.ReadFile(p)
	w.Write(buf)
}

func serve(pattern string, f func(*types.Response, *types.Request)) {
	servedPaths[pattern] = true
	h := gziphandler.GzipHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := r.URL
		now := clock.UnixNano()
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("fatal serving %v: %v, trace: %s", url, r, debug.Stack())
			}
		}()

		req := &types.Request{
			Request:     limiter.LimitRequestSize(r, types.Config.RequestMaxSize),
			ServerStart: serverStart,
			Start:       now,
			Config:      dal.GetJsonizedNoteCache("ns:config"),
			RemoteIPv4:  types.RemoteIPv4(r),
			ServeUUID:   serveUUID,
		}

		resp := &types.Response{ResponseWriter: w}

		if s, created := req.ParseSession(); created {
			http.SetCookie(w, &http.Cookie{
				Name:   "session",
				Value:  s,
				Path:   "/",
				MaxAge: 365 * 86400,
			})
		}

		f(resp, req)

		if el := (clock.UnixNano() - now) / 1e3; r.URL.Path != "/" && servedPaths[r.URL.Path] {
			dal.MetricsIncr(r.URL.Path[1:], float64(el))
		} else {
			dal.MetricsIncr("ns:view", float64(el))
		}
		dal.MetricsIncr("ns:outbound", float64(resp.Written))
	}))
	http.Handle(pattern, h)
}

type actionData struct {
	title, content, image string
	imageChanged          bool
	imageTotal            int
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

		seed := clock.UnixNano()
		ad.imageChanged = q.Get("image_changed") == "true"
		ad.imageTotal, _ = strconv.Atoi(q.Get("image_total"))

		if q.Get("image_small") == "true" {
			ad.image, msg = saveImage(r, id, seed, "s"+ext, img, hdr)
			if msg != "" {
				return ad, msg
			}
		} else {
			thumb, thhdr, _ := r.Request.FormFile("thumb")
			if thumb == nil || thhdr == nil {
				return ad, "INVALID_IMAGE"
			}
			ad.image, msg = saveImage(r, id, seed, "f"+ext, img, hdr)
			if msg != "" {
				return ad, msg
			}
			_, msg = saveImage(r, id, seed, "f.thumb.jpg", thumb, thhdr)
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
			id, ok := clock.Base40Decode(key.Str)
			if ok {
				ad.parentIds = append(ad.parentIds, id)
			}
			return true
		})
		ad.parentIds = types.DedupUint64(ad.parentIds)
		if len(ad.parentIds) > r.MaxParents() {
			return ad, "TOO_MANY_PARENTS"
		}
		ad.parentIds, _ = dal.FilterInvalidParentIds(ad.parentIds)
	}

	ad.hash = buildBitmapHashes(ad.title, "", ad.parentIds)
	if len(ad.hash) == 0 {
		return ad, "EMPTY_TITLE"
	}

	if types.UTF16LenExceeds(ad.title, r.MaxTitle()) {
		return ad, "TITLE_TOO_LONG"
	}

	ad.content = strings.TrimSpace(q.Get("content"))
	if types.UTF16LenExceeds(ad.content, r.MaxContent()) {
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
				id, _ := clock.Base40Decode((query)[6:idx])
				parentIds = append(parentIds, id)
				query = strings.TrimSpace((query)[idx+1:])
			} else {
				id, _ := clock.Base40Decode((query)[6:])
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

type collector struct {
	suggest bool
}

func (col collector) get(q string, parentIds []uint64, uid string) ([]uint64, []bitmap.JoinMetrics) {
	h := ngram.SplitMore(q).Hashes()

	var h2 []uint64
	var allLetter = true
	for _, r := range q {
		if unicode.IsLower(r) || unicode.IsUpper(r) {
		} else {
			allLetter = false
			break
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

	max := 2000
	if len(h2)+len(h) <= 2 {
		max = 200
	}

	in := bitmap.Values{Major: h2, Exact: h}
	if col.suggest {
		in = bitmap.Values{Oneof: h2}
	}

	res, jms := dal.Store.CollectSimple(cursor.New(), in, max)
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
