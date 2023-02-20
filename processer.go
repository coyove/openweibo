package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
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
	"github.com/coyove/sdss/contrib/simple"
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
	"generatePages": func(r *types.Request, suffix string) string {
		p, pages := int64(r.P.Page), reflect.ValueOf(r.T["pages"]).Int()
		var a []int64
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
		buf := bytes.NewBufferString("<div id=page>")
		for _, a := range a {
			if a == 0 {
				buf.WriteString("<div style='width:2em;text-align:center'>&middot;&middot;&middot;</div>")
			} else if a == p {
				fmt.Fprintf(buf, "<div class='tag-box button selected'><span><b>%d</b></span></div>", a)
			} else {
				fmt.Fprintf(buf, "<a class=nav-page href='?%s%s'><div class='tag-box button'>%d</div></a>",
					r.BuildPageLink(int(a)), suffix, a)
			}
		}
		buf.WriteString("</div>")
		return buf.String()
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
	"makeTitle": func(t *types.Note, max int) string {
		if t.IsBio {
			return "<b>个人记事</b>"
		}
		tt := types.SafeHTML(t.Title)
		if tt == "" {
			notes, _ := dal.BatchGetNotes(t.ParentIds)
			if len(notes) > 0 {
				sort.Slice(notes, func(i, j int) bool {
					if len(notes[i].Title) == len(notes[j].Title) {
						return notes[i].ChildrenCount > notes[j].ChildrenCount
					}
					return len(notes[i].Title) > len(notes[j].Title)
				})
				buf, c := &bytes.Buffer{}, 0
				for _, n := range notes {
					if tt := types.SafeHTML(n.Title); tt == "" {
						fmt.Fprintf(buf, "<span style='font-style:italic'><b>#</b>ns:id:%s</span> ", t.IdStr())
						c += 16
					} else {
						fmt.Fprintf(buf, "<span><b>#</b>%s</span> ", tt)
						c += len(tt)
					}
					if c > max {
						break
					}
				}
				return buf.String()
			}
			if t.Image != "" && t.IsImage() {
				return fmt.Sprintf("%s (%.2fM)", t.FileExt(), float64(t.FileSize())/1024/1024)
			}
			return t.HTMLTitleDisplay()
		}
		return tt
	},
	"mega": func(in interface{}) string {
		return fmt.Sprintf("%.2fM", float64(reflect.ValueOf(in).Int())/1024/1024)
	},
	"add":          func(a, b int) int { return a + b },
	"uuid":         func() string { return types.UUIDStr() },
	"imageURL":     imageURL,
	"equ64s":       simple.Uint64.Equal,
	"trunc":        types.UTF16Trunc,
	"renderClip":   types.RenderClip,
	"safeHTML":     types.SafeHTML,
	"fullEscape":   types.FullEscape,
	"base62Encode": clock.Base62Encode,
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
		req.ParseSession(resp, func(id string) *types.User {
			u, _ := dal.GetUser(id)
			return u
		})
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
		ext := strings.ToLower(filepath.Ext(filepath.Base(hdr.Filename)))
		if ext == "" {
			return ad, "INVALID_IMAGE_NAME"
		}

		seed := clock.UnixNano()
		ad.imageChanged = q.Get("image_changed") == "true"
		ad.imageTotal, _ = strconv.Atoi(q.Get("image_total"))
		fileSize, _ := strconv.Atoi(q.Get("file_size"))

		var totalSize, tmpSize int64
		if ft := q.Get("file_type"); !strings.HasPrefix(ft, "image/") {
			tmpSize, ad.image, msg = saveFile(r, id, seed, "a"+ext, fileSize, img, hdr)
			if msg != "" {
				return ad, msg
			}
			totalSize += tmpSize
		} else if q.Get("image_small") == "true" {
			tmpSize, ad.image, msg = saveFile(r, id, seed, "s"+ext, fileSize, img, hdr)
			if msg != "" {
				return ad, msg
			}
			totalSize += tmpSize
		} else {
			thumb, thhdr, _ := r.Request.FormFile("thumb")
			if thumb == nil || thhdr == nil {
				return ad, "INVALID_IMAGE"
			}
			tmpSize, ad.image, msg = saveFile(r, id, seed, "f"+ext, fileSize, img, hdr)
			if msg != "" {
				return ad, msg
			}
			totalSize += tmpSize

			tmpSize, _, msg = saveFile(r, id, seed, "f.thumb.jpg", fileSize, thumb, thhdr)
			if msg != "" {
				return ad, msg
			}
			totalSize += tmpSize
		}
		dal.UpdateUser(r.User.Id, func(u *types.User) error {
			u.UploadSize += totalSize
			return nil
		})
	}

	ad.title = types.CleanTitle(q.Get("title"))

	if strings.HasPrefix(ad.title, "ns:") && !r.User.IsRoot() {
		return ad, "MODS_REQUIRED"
	}

	if pt := q.Get("parents"); pt != "" {
		gjson.Parse(pt).ForEach(func(key, value gjson.Result) bool {
			id, ok := clock.Base62Decode(key.Str)
			if ok {
				ad.parentIds = append(ad.parentIds, id)
			} else if !ok {
				ad.parentIds = append(ad.parentIds, key.Uint())
			}
			return true
		})
		ad.parentIds = simple.Uint64.Dedup(ad.parentIds)
		if len(ad.parentIds) > r.MaxParents() {
			return ad, "TOO_MANY_PARENTS"
		}
		ad.parentIds, _ = dal.FilterInvalidParentIds(ad.parentIds)
	}

	ad.hash = buildBitmapHashes(ad.title, "", ad.parentIds)
	if len(ad.hash) == 0 {
		note, _ := dal.GetNote(id)
		if note.Valid() && note.IsBio {
		} else {
			return ad, "EMPTY_TITLE"
		}
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
				id, _ := clock.Base62Decode((query)[6:idx])
				parentIds = append(parentIds, id)
				query = strings.TrimSpace((query)[idx+1:])
			} else {
				id, _ := clock.Base62Decode((query)[6:])
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

func extractVideoThumb(src, dest string) error {
	out := &bytes.Buffer{}
	e := "./ffmpeg"
	if runtime.GOOS == "windows" {
		e = "./ffmpeg.exe"
	}
	cmd := exec.Command(e,
		"-i", src, "-vf", `select=eq(n\,50)`, "-q:v", "3", "-vframes", "1", "-hide_banner", "-loglevel", "error", dest)
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return err
	}
	if out.Len() > 0 {
		return errors.New(out.String())
	}
	return nil
}
