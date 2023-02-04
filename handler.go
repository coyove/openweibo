package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/limiter"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/sirupsen/logrus"
)

func HandleAction(w *types.Response, r *types.Request) {
	if ok, remains := limiter.CheckIP(r); !ok {
		if remains == -1 {
			w.WriteJSON("success", false, "code", "IP_BANNED")
		} else {
			w.WriteJSON("success", false, "code", "COOLDOWN", "remains", remains)
		}
		return
	}

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

	var target *types.Note
	var ok bool
	if action == "create" {
		target, ok = doCreate(w, r)
	} else if action == "update" {
		target, ok = doUpdate(w, r)
	} else {
		id, err := strconv.ParseUint(r.Form.Get("id"), 10, 64)

		defer func(start time.Time) {
			logrus.Infof("action %q - %d in %v", action, id, time.Since(start))
		}(time.Now())

		dal.LockKey(id)
		defer dal.UnlockKey(id)

		target, err = dal.GetNote(id)
		if !target.Valid() || err != nil {
			logrus.Errorf("action %q, can't find %d: %v", action, id, err)
			w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
			return
		}

		if action == "touch" {
			ok = doTouch(target, w, r)
		} else if action == "approve" {
			ok = doApprove(target, false, w, r)
		} else if action == "reject" {
			ok = doReject(target, w, r)
		} else if action == "delete" {
			ok = doDelete(target, w, r)
		} else if action == "Lock" || action == "Unlock" {
			ok = doLockUnlock(target, action == "Lock", w, r)
		} else {
			w.WriteJSON("success", false, "code", "INVALID_ACTION")
			return
		}
	}
	if ok {
		if action != "touch" {
			go dal.AppendHistory(target.Id, r.UserDisplay, action,
				types.UTF16Trunc(r.Form.Get("reject_msg"), 100),
				r.RemoteIPv4, target)
		}
		limiter.AddIP(r)
		w.WriteJSON("success", true, "note", target)
	}
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
		q, parentIds, uid := expandQuery(q)
		ids, jms = (collector{}).get(q, parentIds, uid)
		dn, _ := dal.GetNoteByName(q)
		if dn.Valid() {
			ids = types.DedupUint64(append(ids, dn.Id))
		}
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
		tt := t.Title
		if tt == "" {
			tt = "ns:id:" + strconv.FormatUint(t.Id, 10)
		}
		results = append(results, [3]interface{}{t.Id, tt, t.ChildrenCount})
		return len(results) < n
	})
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

func HandleNew(w *types.Response, r *types.Request) {
	var parents []string
	var lastParent uint64
	var lastTitle string
	for _, p := range types.SplitUint64List(r.URL.Query().Get("parents")) {
		if note, _ := dal.GetNote(p); note.Valid() {
			t := note.Title
			if t == "" {
				parents = append(parents, fmt.Sprintf("%d,ns:id:%d", note.Id, note.Id))
			} else {
				parents = append(parents, fmt.Sprintf("%d,%s", note.Id, t))
			}
			lastParent = p
			lastTitle = t
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
	_ = uq
	// if p := uq.Get("prefix"); p != "" {
	// 	page := dal.KSVPagingFindLexPrefix(fmt.Sprintf("children_%d", note.Id), []byte(p), r.P.Desc, r.P.PageSize)
	// 	http.Redirect(w, r.Request, fmt.Sprintf("/ns:id:%d?sort=1&desc=%v&p=%d", note.Id, r.P.Desc, page), 302)
	// 	return
	// }

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
			// if p := uq.Get("prefix"); p != "" {
			// 	page := dal.KSVPagingFindLexPrefix("creator_"+uid, []byte(p), r.P.Desc, r.P.PageSize)
			// 	http.Redirect(w, r.Request, fmt.Sprintf("/ns:manage?sort=1&desc=%v&p=%d&q=%s", r.P.Desc, page, url.QueryEscape(q)), 302)
			// 	return
			// }

			results, total, pages = dal.KSVPaging(nil, "creator_"+uid, r.P.Sort, r.P.Desc, r.P.Page-1, r.P.PageSize)
			notes, _ = dal.BatchGetNotes(results)
			r.AddTemplateValue("isUserPage", true)
			r.AddTemplateValue("viewUser", uid)
		} else if len(pids) == 1 && pids[0] > 0 && uid == "" && query == "" {
			http.Redirect(w, r.Request, "/ns:id:"+strconv.FormatUint(pids[0], 10), 302)
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
		// if p := uq.Get("prefix"); p != "" {
		// 	page := dal.KSVPagingFindLexPrefix(dal.NoteBK, []byte(p), r.P.Desc, r.P.PageSize)
		// 	http.Redirect(w, r.Request, fmt.Sprintf("/ns:manage?sort=1&desc=%v&p=%d", r.P.Desc, page), 302)
		// 	return
		// }

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
