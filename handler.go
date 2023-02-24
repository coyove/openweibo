package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/limiter"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/coyove/sdss/contrib/simple"
	"github.com/sirupsen/logrus"
)

func gerr(e error) string {
	if e, ok := e.(writeError); ok {
		return string(e)
	}
	return "INTERNAL_ERROR"
}

func HandleAction(w *types.Response, r *types.Request) {
	if ok, remains := limiter.CheckIP(r); !ok {
		if remains == -1 {
			w.WriteJSON("success", false, "code", "IP_BANNED")
		} else {
			w.WriteJSON("success", false, "code", "COOLDOWN", "remains", remains)
		}
		return
	}

	defer func() {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
	}()

	if err := r.ParseMultipartForm(types.Config.RequestMaxSize); err != nil {
		if err == limiter.ErrRequestTooLarge {
			w.WriteJSON("success", false, "code", "CONTENT_TOO_LARGE")
		} else {
			logrus.Errorf("failed to parse multipart: %v", err)
			w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
		}
		return
	}

	action := r.Header.Get("X-Ns-Action")
	if action == "preview" {
		doPreview(w, r)
		return
	}
	if strings.HasPrefix(action, "user:") {
		doUserAction(action, w, r)
		return
	}

	if r.User == nil {
		w.WriteJSON("success", false, "code", "PLEASE_LOGIN")
		return
	}

	id, _ := strconv.ParseUint(r.Form.Get("id"), 10, 64)

	var err error

	start := time.Now()
	switch action {
	case "approve":
		err = doApprove(id, r)
	case "create":
		id = clock.Id()
		err = doCreate(id, r)
	case "delete":
		err = doDeleteHide(false, r)
	case "hide":
		err = doDeleteHide(true, r)
	case "Lock":
		err = doLockUnlock(id, true, r)
	case "reject":
		err = doReject(id, r)
	case "touch":
		err = doTouch(id, r)
	case "update":
		err = doUpdate(id, r)
	case "Unlock":
		err = doLockUnlock(id, false, r)
	default:
		err = writeError("INVALID_ACTION")
	}
	if err != nil {
		logrus.Errorf("action %q %v: %v", action, id, err)
		w.WriteJSON("success", false, "code", gerr(err))
		return
	}
	logrus.Infof("action %q %v in %v", action, id, time.Since(start))

	if action != "touch" && action != "hide" {
		msg := types.UTF16Trunc(r.Form.Get("reject_msg"), 100)
		go dal.AppendHistory(id, r.UserDisplay, action, msg, r.RemoteIPv4.String())
	}

	limiter.AddIP(r)
	w.WriteJSON("success", true, "note_id", strconv.FormatUint(id, 10))
}

func doUserAction(action string, w *types.Response, r *types.Request) {
	switch action {
	case "user:login", "user:register":
		u, err := doUserLogin(action == "user:register", r)
		if err != nil {
			w.WriteJSON("success", false, "code", gerr(err))
		} else {
			http.SetCookie(w, u.GenerateSession())
			w.WriteJSON("success", true, "user_id", u.Id)
		}
		limiter.AddIP(r)
		return
	}

	if r.User == nil {
		w.WriteJSON("success", false, "code", "PLEASE_LOGIN")
		return
	}

	var err error
	start := time.Now()
	switch action {
	case "user:logout":
		if _, err = dal.UpdateUser(r.User.Id, func(u *types.User) error {
			u.Session64 = rand.Int63()
			return nil
		}); err == nil {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/"})
		}
	case "user:resetpwd":
		err = doResetPassword(r)
	case "user:update":
		var out *types.User
		if _, err = dal.UpdateUser(r.User.Id, func(u *types.User) error {
			u.Email = types.UTF16Trunc(r.Form.Get("email"), 100)
			u.HideImage = r.Form.Get("hide_image") == "true"
			out = u
			return nil
		}); err == nil {
			http.SetCookie(w, out.GenerateSession())
		}
	case "user:updatepwd":
		if err = doUpdatePassword(r); err == nil {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/"})
		}
	default:
		err = writeError("INVALID_ACTION")
	}

	limiter.AddIP(r)
	if err != nil {
		logrus.Errorf("user action %q %v: %v", action, r.User.Id, err)
		w.WriteJSON("success", false, "code", gerr(err))
		return
	}
	logrus.Infof("user action %q %v in %v", action, r.User.Id, time.Since(start))
	w.WriteJSON("success", true)
}

func HandleHistory(w *types.Response, r *types.Request) {
	uq := r.ParsePaging()

	view := "all"
	idStr := uq.Get("note")
	if idStr != "" {
		view = "note"
	} else {
		idStr = uq.Get("user")
		if idStr != "" {
			view = "user"
		}
	}

	var results []dal.KeySortValue
	var records []*types.Record
	var note = &types.Note{}
	var total, pages int

	switch view {
	default:
		results, total, pages = dal.KSVPaging(nil, "history", -1, r.P.Desc, r.P.Page-1, r.P.PageSize)
		view, idStr = "all", ""
	case "note":
		id, _ := strconv.ParseUint(idStr, 10, 64)
		results, total, pages = dal.KSVPaging(nil, fmt.Sprintf("history_%d", id), -1, r.P.Desc, r.P.Page-1, r.P.PageSize)
		tmp, _ := dal.GetNote(id)
		if tmp.Valid() {
			note = tmp
		}
	case "user":
		results, total, pages = dal.KSVPaging(nil, "history_"+idStr, -1, r.P.Desc, r.P.Page-1, r.P.PageSize)
	}

	for i := range results {
		var t *types.Record
		if view == "all" {
			t = types.UnmarshalRecordBinary(results[i].Value)
		} else {
			t, _ = dal.GetTagRecord(types.BytesUint64(results[i].Key))
			if t == nil || t.Note() == nil {
				continue
			}
			if view == "note" && !note.Valid() && t.ActionName() != "delete" {
				note = t.Note()
			}
		}
		records = append(records, t)
	}

	r.AddTemplateValue("id", idStr)
	r.AddTemplateValue("query", "")
	r.AddTemplateValue("viewType", view)
	r.AddTemplateValue("note", note)
	r.AddTemplateValue("records", records)
	r.AddTemplateValue("total", total)
	r.AddTemplateValue("pages", pages)
	httpTemplates.ExecuteTemplate(w, "history.html", r)
}

func HandleTagSearch(w *types.Response, r *types.Request) {
	start, uq := time.Now(), r.URL.Query()
	q := types.UTF16Trunc(uq.Get("q"), 50)
	n, _ := strconv.Atoi(uq.Get("n"))
	if n == 0 {
		n = 100
	}
	n = imin(100, n)
	n = imax(1, n)

	var ids []uint64
	var jms []bitmap.JoinMetrics

	if noteIDs := uq.Get("ids"); noteIDs != "" {
		ids = types.SplitUint64List(noteIDs)
	} else if name := uq.Get("title"); name != "" {
		note, _ := dal.GetNoteByName(types.CleanTitle(name))
		if note.Valid() {
			ids = append(ids, note.Id)
		}
	} else {
		ids, jms = (collector{}).get(q, nil, "")
	}

	notes, _ := dal.BatchGetNotes(ids)
	sort.Slice(notes, func(i, j int) bool {
		if len(notes[i].Title) == len(notes[j].Title) {
			return notes[i].ChildrenCount+notes[i].TouchCount > notes[j].ChildrenCount+notes[j].TouchCount
		}
		return len(notes[i].Title) < len(notes[j].Title)
	})

	results := [][3]interface{}{}
	doubleCheckFilterResults(q, nil, notes, func(t *types.Note) bool {
		results = append(results, [3]interface{}{t.IdStr(), t.TitleDisplay(), t.ChildrenCount})
		return len(results) < n
	})

	if q != "" {
		if dn, _ := dal.GetNoteByName(q); dn.Valid() && !simple.Uint64.Contains(ids, dn.Id) {
			results = append([][3]interface{}{{dn.IdStr(), dn.TitleDisplay(), dn.ChildrenCount}}, results...)
		}
	}

	diff := time.Since(start)
	w.WriteJSON(
		"success", true,
		"notes", results,
		"elapsed", diff.Milliseconds(),
		"elapsed_us", diff.Microseconds(),
		"debug", fmt.Sprint(jms),
		"count", len(results),
	)
}

func HandleUser(w *types.Response, r *types.Request) {
	uq := r.ParsePaging()
	viewUser := func(r *types.Request, id string) {
		u, _ := dal.GetUser(id)
		r.AddTemplateValue("user", u)
		r.AddTemplateValue("userCreates", dal.KSVCount(nil, "creator_"+id))

		results, total, pages := dal.KSVPaging(nil, dal.UserBK, r.P.Sort, r.P.Desc, r.P.Page-1, r.P.PageSize)
		users := make([]*types.User, len(results))
		for i := range users {
			users[i] = types.UnmarshalUserBinary(results[i].Value)
		}
		r.AddTemplateValue("users", users)
		r.AddTemplateValue("total", total)
		r.AddTemplateValue("pages", pages)
	}

	if r.User != nil {
		if id := uq.Get("id"); id != "" && r.User.IsMod() {
			viewUser(r, id)
		} else {
			viewUser(r, r.User.Id)
		}
	}
	httpTemplates.ExecuteTemplate(w, "user.html", r)
}

func HandleNew(w *types.Response, r *types.Request) {
	var parents []uint64 = types.SplitUint64List(r.URL.Query().Get("parents"))
	var lastParent, lastTitle string
	for _, p := range parents {
		if note, _ := dal.GetNote(p); note.Valid() {
			lastParent = note.IdStr()
			lastTitle = note.Title
			break
		}
	}
	r.AddTemplateValue("parents", parents)
	r.AddTemplateValue("lastParent", lastParent)
	r.AddTemplateValue("lastTitle", lastTitle)
	httpTemplates.ExecuteTemplate(w, "new.html", r)
}

func HandleEdit(w *types.Response, r *types.Request) {
	var note *types.Note
	var readonly string
	var recordUnix int64
	id, _ := strconv.ParseUint(r.URL.Query().Get("id"), 10, 64)
	if id > 0 {
		note, _ = dal.GetNote(id)
	}
	hid, _ := strconv.ParseUint(r.URL.Query().Get("hid"), 10, 64)
	if hid > 0 {
		r, _ := dal.GetTagRecord(hid)
		if r != nil {
			note = r.Note()
			recordUnix = r.CreateUnix
			readonly = "readonly"
		}
	}
	if !note.Valid() {
		http.Redirect(w, r.Request, "/ns:not_found", 302)
		return
	}
	r.AddTemplateValue("note", note)
	r.AddTemplateValue("readonly", readonly)
	r.AddTemplateValue("recordUnix", recordUnix)
	httpTemplates.ExecuteTemplate(w, "edit.html", r)
}

func HandleView(w *types.Response, r *types.Request) {
	t := strings.TrimPrefix(r.URL.Path, "/")
	if t == "" {
		t = "ns:welcome"
	}

	note, _ := dal.GetNoteByName(t)
	if !note.Valid() {
		if strings.HasPrefix(t, "ns:id:") {
			http.Redirect(w, r.Request, "/ns:not_found", 302)
		} else {
			http.Redirect(w, r.Request, "/ns:manage?q="+types.FullEscape(t), 302)
		}
		return
	}

	var notes []*types.Note
	if len(note.ParentIds) > 0 {
		notes, _ = dal.BatchGetNotes(note.ParentIds)
	}

	uq := r.ParsePaging()
	if p, _ := strconv.ParseUint(uq.Get("findid"), 10, 64); p > 0 {
		page := dal.KSVPagingSort0FindKey(fmt.Sprintf("children_%d", note.Id), types.Uint64Bytes(p), r.P.Desc, r.P.PageSize)
		http.Redirect(w, r.Request, fmt.Sprintf("/ns:id:%s?sort=0&desc=%v&p=%d#note%d", note.IdStr(), r.P.Desc, page, p), 302)
		return
	}

	results, total, pages := dal.KSVPaging(nil, fmt.Sprintf("children_%d", note.Id), r.P.Sort, r.P.Desc, r.P.Page-1, r.P.PageSize)
	children, _ := dal.BatchGetNotes(results)

	r.AddTemplateValue("view", true)
	r.AddTemplateValue("query", "")
	r.AddTemplateValue("notes", children)
	r.AddTemplateValue("total", total)
	r.AddTemplateValue("pages", pages)

	if r.User.IsRoot() {
		a, b := checkImageCache(note)
		r.AddTemplateValue("checkImagesFound", a)
		r.AddTemplateValue("checkImagesUploaded", b)
	}

	if note.Image != "" || note.Content != "" {
		views, _ := dal.MetricsSetAdd(strconv.FormatUint(note.Id, 10), r.UserDisplay)
		r.AddTemplateValue("noteViews", views)
	} else {
		r.AddTemplateValue("noteViews", 0)
	}

	r.AddTemplateValue("noteTouched", dal.KSVExist(nil, "creator_"+r.UserDisplay, types.Uint64Bytes(note.Id)))
	r.AddTemplateValue("note", note)
	r.AddTemplateValue("parents", notes)
	httpTemplates.ExecuteTemplate(w, "manage.html", r)
}

func HandleManage(w *types.Response, r *types.Request) {
	uq := r.ParsePaging()
	q := types.CleanTitle(uq.Get("q"))
	r.AddTemplateValue("query", q)

	var notes []*types.Note
	var results []dal.KeySortValue
	var total, pages int

	if q != "" {
		query, pids, uid := expandQuery(q)
		if len(pids) == 0 && query == "" && uid != "" {
			user, _ := dal.GetUser(uid)
			if !user.Valid() {
				http.Redirect(w, r.Request, "/ns:not_found", 302)
				return
			}
			results, total, pages = dal.KSVPaging(nil, "creator_"+uid, r.P.Sort, r.P.Desc, r.P.Page-1, r.P.PageSize)
			notes, _ = dal.BatchGetNotes(results)
			r.AddTemplateValue("isUserPage", true)
			r.AddTemplateValue("viewUser", user)
			r.AddTemplateValue("viewUserId", uid)
		} else if len(pids) == 1 && pids[0] > 0 && uid == "" && query == "" {
			http.Redirect(w, r.Request, "/ns:id:"+clock.Base62Encode(pids[0]), 302)
			return
		} else {
			ids, _ := (collector{}).get(query, pids, uid)
			res, _ := dal.BatchGetNotes(ids)
			total = len(res)
			res = sortNotes(res, r.P.Sort, r.P.Desc)
			doubleCheckFilterResults(query, pids, res, func(t *types.Note) bool {
				notes = append(notes, t)
				return len(notes) < 500
			})
			r.AddTemplateValue("isSearchPage", true)
		}
	} else {
		results, total, pages = dal.KSVPaging(nil, dal.NoteBK, r.P.Sort, r.P.Desc, r.P.Page-1, r.P.PageSize)
		notes = make([]*types.Note, len(results))
		for i := range notes {
			notes[i] = types.UnmarshalNoteBinary(results[i].Value)
		}
	}

	r.AddTemplateValue("view", false)
	r.AddTemplateValue("notes", notes)
	r.AddTemplateValue("total", total)
	r.AddTemplateValue("pages", pages)
	httpTemplates.ExecuteTemplate(w, "manage.html", r)
}
