package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/coyove/sdss/contrib/ngram"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"go.etcd.io/bbolt"
)

func HandleTagAction(w http.ResponseWriter, r *types.Request) {
	if !dal.CheckIP(r.RemoteIPv4) {
		writeJSON(w, "success", false, "code", "IP_BANNED")
		return
	}

	r.ParseMultipartForm(10 * 1024 * 1024)
	q := r.Form
	id, err := strconv.ParseUint(q.Get("id"), 10, 64)
	action := q.Get("action")
	idKey := types.Uint64Bytes(id)

	defer func(start time.Time) {
		logrus.Infof("action %q - %d, in %v", action, id, time.Since(start))
	}(time.Now())

	var target *types.Note
	if action != "create" {
		dal.LockKey(id)
		defer dal.UnlockKey(id)
		target, err = dal.GetNote(id)
		if !target.Valid() || err != nil {
			logrus.Errorf("action %q, can't find %d: %v", action, id, err)
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
			return
		}
	}

	switch action {
	case "Lock", "Unlock", "delete":
		if !r.User.IsMod() {
			writeJSON(w, "success", false, "code", "MODS_REQUIRED")
			return
		}
	}

	switch action {
	case "update":
		if target.PendingReview {
			if target.Modifier == r.UserDisplay || target.Creator == r.UserDisplay || r.User.IsMod() {
				// Creator, modifier and moderators can still update the content.
			} else {
				writeJSON(w, "success", false, "code", "PENDING_REVIEW")
				return
			}
		}
		if target.Lock && !r.User.IsMod() {
			writeJSON(w, "success", false, "code", "LOCKED")
			return
		}
		fallthrough
	case "create":
		title := cleanTitle(q.Get("title"))
		content := strings.TrimSpace(q.Get("content"))
		h := buildBitmapHashes(title)
		if len(title) < 1 ||
			types.UTF16LenExceeds(title, r.GetTitleMaxLen()) ||
			len(h) == 0 ||
			types.UTF16LenExceeds(content, r.GetContentMaxLen()) {
			writeJSON(w, "success", false, "code", "INVALID_CONTENT")
			return
		}
		if strings.HasPrefix(title, "ns:") && !r.User.IsMod() {
			writeJSON(w, "success", false, "code", "MODS_REQUIRED")
			return
		}

		var parentIds []uint64
		if pt := q.Get("parents"); pt != "" {
			gjson.Parse(pt).ForEach(func(key, value gjson.Result) bool {
				parentIds = append(parentIds, key.Uint())
				return true
			})
			if len(parentIds) > 8 {
				writeJSON(w, "success", false, "code", "TOO_MANY_PARENTS")
				return
			}
		}

		var err error
		var exist, shouldIndex bool
		if action == "create" {
			target = &types.Note{
				Id:        clock.Id(),
				Title:     title,
				Content:   content,
				Creator:   r.UserDisplay,
				ParentIds: parentIds,
			}
			id, idKey = target.Id, types.Uint64Bytes(target.Id)
			exist, err = dal.CreateNote(title, target)
			shouldIndex = true
		} else {
			err = dal.Store.Update(func(tx *bbolt.Tx) error {
				if title != target.Title {
					if _, exist = dal.KSVFirstKeyOfSort1(tx, dal.NoteBK, []byte(title)); exist {
						return nil
					}
				}
				dal.ProcessTagParentChanges(tx, target, target.ParentIds, parentIds)
				target.ParentIds = parentIds
				if r.User.IsMod() || r.UserDisplay == target.Creator {
					shouldIndex = title != target.Title
					target.Title = title
					target.Content = content
				} else {
					target.PendingReview = true
					target.ReviewTitle = title
					target.ReviewContent = content
				}
				target.Modifier = r.UserDisplay
				target.UpdateUnix = clock.UnixMilli()
				dal.UpdateCreator(tx, target)
				return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
			})
		}
		if err != nil {
			logrus.Errorf("tag manage action %s %d: %v", action, id, err)
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
			return
		}
		if exist {
			writeJSON(w, "success", false, "code", "DUPLICATED_TITLE")
			return
		}
		if shouldIndex {
			dal.Store.Saver().AddAsync(bitmap.Uint64Key(id), h)
		}
	case "delete":
		if err := dal.Store.Update(func(tx *bbolt.Tx) error {
			dal.ProcessTagParentChanges(tx, target, target.ParentIds, nil)
			return dal.KSVDelete(tx, dal.NoteBK, idKey[:])
		}); err != nil {
			logrus.Errorf("tag manage action %s %d: %v", action, id, err)
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
			return
		}
	case "approve", "reject":
		if !target.PendingReview {
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
			return
		}
		var shouldIndex bool
		if action == "approve" {
			if !r.User.IsMod() && target.Creator != r.UserDisplay {
				writeJSON(w, "success", false, "code", "ILLEGAL_APPROVE")
				return
			}
			shouldIndex = target.Title != target.ReviewTitle
			target.Title = target.ReviewTitle
			target.Content = target.ReviewContent
			target.Reviewer = r.UserDisplay
		} else {
			if !r.User.IsMod() && target.Creator != r.UserDisplay && target.Modifier != r.UserDisplay {
				writeJSON(w, "success", false, "code", "ILLEGAL_APPROVE")
				return
			}
		}
		target.PendingReview = false
		target.ReviewTitle, target.ReviewContent = "", ""

		var exist bool
		if err := dal.Store.Update(func(tx *bbolt.Tx) error {
			if target.Title == "" && action == "reject" {
				return dal.KSVDelete(tx, dal.NoteBK, idKey[:])
			}
			if key, found := dal.KSVFirstKeyOfSort1(tx, dal.NoteBK, []byte(target.Title)); found {
				if !bytes.Equal(key, idKey[:]) {
					exist = true
					return nil
				}
			}
			dal.UpdateCreator(tx, target)
			return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
		}); err != nil {
			logrus.Errorf("tag manage action %s %d: %v", action, id, err)
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
			return
		}
		if exist {
			writeJSON(w, "success", false, "code", "DUPLICATED_TITLE")
			return
		}
		if shouldIndex {
			dal.Store.Saver().AddAsync(bitmap.Uint64Key(id), buildBitmapHashes(target.Title))
		}
	case "Lock", "Unlock":
		target.Lock = action == "Lock"
		target.UpdateUnix = clock.UnixMilli()
		if err := dal.Store.Update(func(tx *bbolt.Tx) error {
			return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
		}); err != nil {
			logrus.Errorf("tag manage action %s %d: %v", action, id, err)
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
			return
		}
	case "like":
		if err := dal.Store.Update(func(tx *bbolt.Tx) error {
			return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
		}); err != nil {
			logrus.Errorf("tag manage action %s %d: %v", action, id, err)
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
			return
		}
	default:
		writeJSON(w, "success", false, "code", "INVALID_ACTION")
		return
	}

	go dal.ProcessTagHistory(target.Id, r.UserDisplay, action, r.RemoteIPv4Masked(), target)
	writeJSON(w, "success", true, "note", target)
}

func HandleManage(w http.ResponseWriter, r *types.Request) {
	p, st, desc, pageSize := r.GetPagingArgs()
	q := r.URL.Query().Get("q")
	pidStr := r.URL.Query().Get("pid")
	if pidStr == "0" {
		pidStr = ""
	}

	var notes []*types.Note
	var total, pages int
	if q != "" {
		st, desc = -1, false
		ids, _ := collectSimple(q)
		var tmp []uint64
		for _, id := range ids {
			tmp = append(tmp, id.LowUint64())
		}
		notes, _ = dal.BatchGetNotes(tmp)
		total = len(notes)
		sort.Slice(notes, func(i, j int) bool { return len(notes[i].Title) < len(notes[j].Title) })
		notes = notes[:imin(500, len(notes))]
	} else {
		var results []dal.KeySortValue
		if pidStr == "" {
			results, total, pages = dal.KSVPaging(nil, dal.NoteBK, st, desc, p-1, pageSize)
			notes = make([]*types.Note, len(results))
			for i := range notes {
				notes[i] = types.UnmarshalTagBinary(results[i].Value)
			}
		} else {
			results, total, pages = dal.KSVPaging(nil, "creator_"+pidStr, st, desc, p-1, pageSize)
			notes, _ = dal.BatchGetNotes(results)
		}
	}

	r.AddTemplateValue("query", q)
	r.AddTemplateValue("pid", pidStr)
	r.AddTemplateValue("tags", notes)
	r.AddTemplateValue("total", total)
	r.AddTemplateValue("pages", pages)
	httpTemplates.ExecuteTemplate(w, "manage.html", r)
}

func HandleHistory(w http.ResponseWriter, r *types.Request) {
	p, _, desc, pageSize := r.GetPagingArgs()

	view := "all"
	idStr := r.URL.Query().Get("note")
	if idStr != "" {
		view = "note"
	} else {
		idStr = r.URL.Query().Get("user")
		if idStr != "" {
			view = "user"
		}
	}

	var results []dal.KeySortValue
	var records []*types.NoteRecord
	var note = &types.Note{}
	var total, pages int

	switch view {
	default:
		results, total, pages = dal.KSVPaging(nil, "history", -1, desc, p-1, pageSize)
		view, idStr = "all", ""
	case "note":
		id, _ := strconv.ParseUint(idStr, 10, 64)
		if note, _ = dal.GetNote(id); note.Valid() {
			results, total, pages = dal.KSVPaging(nil, fmt.Sprintf("history_%d", note.Id), -1, desc, p-1, pageSize)
		} else {
			http.Redirect(w, r.Request, "/notfound", 302)
			return
		}
	case "user":
		results, total, pages = dal.KSVPaging(nil, "history_"+idStr, -1, desc, p-1, pageSize)
	}

	for i := range results {
		var t *types.NoteRecord
		if view == "all" {
			t = types.UnmarshalTagRecordBinary(results[i].Value)
		} else {
			t, _ = dal.GetTagRecord(types.BytesUint64(results[i].Key))
			if t == nil || t.Note() == nil {
				continue
			}
		}
		records = append(records, t)
	}

	r.AddTemplateValue("id", idStr)
	r.AddTemplateValue("viewType", view)
	r.AddTemplateValue("note", note)
	r.AddTemplateValue("total", total)
	r.AddTemplateValue("records", records)
	r.AddTemplateValue("pages", pages)
	httpTemplates.ExecuteTemplate(w, "history.html", r)
}

func HandleTagSearch(w http.ResponseWriter, r *types.Request) {
	start := time.Now()
	q := types.UTF16Trunc(r.URL.Query().Get("q"), 50)
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n == 0 {
		n = 100
	}
	n = imin(100, n)
	n = imax(1, n)

	var ids []uint64
	var jms []bitmap.JoinMetrics

	if tagIDs := r.URL.Query().Get("ids"); tagIDs != "" {
		for _, p := range strings.Split(tagIDs, ",") {
			id, _ := strconv.ParseUint(p, 10, 64)
			ids = append(ids, (id))
		}
	} else {
		var tmp []bitmap.Key
		tmp, jms = collectSimple(q)
		for _, id := range tmp {
			ids = append(ids, (id.LowUint64()))
		}
	}

	tags, _ := dal.BatchGetNotes(ids)
	sort.Slice(tags, func(i, j int) bool { return len(tags[i].Title) < len(tags[j].Title) })

	results := []interface{}{}
	h := ngram.SplitMore(q)
	for i, tag := range tags {
		if i >= n {
			break
		}
		if tag.Title != "" {
			if len(h) == 0 || ngram.SplitMore(tag.Title).Contains(h) {
				results = append(results, [3]interface{}{
					tag.Id,
					tag.Title,
					tag.ParentIds,
				})
			}
		}
	}
	diff := time.Since(start)
	writeJSON(w,
		"success", true,
		"notes", results,
		"elapsed", diff.Milliseconds(),
		"elapsed_us", diff.Microseconds(),
		"debug", fmt.Sprint(jms),
		"count", len(results),
	)
}

func HandleTagNew(w http.ResponseWriter, r *types.Request) {
	httpTemplates.ExecuteTemplate(w, "new.html", r)
}

func HandleEdit(w http.ResponseWriter, r *types.Request) {
	var note *types.Note
	var readonly bool
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
			readonly = true
		}
	}
	if !note.Valid() {
		http.Redirect(w, r.Request, "/notfound", 302)
		return
	}
	r.AddTemplateValue("note", note)
	r.AddTemplateValue("readonly", readonly)
	r.AddTemplateValue("recordUnix", recordUnix)
	httpTemplates.ExecuteTemplate(w, "edit.html", r)
}

func HandleView(w http.ResponseWriter, r *types.Request) {
	t := strings.TrimPrefix(r.URL.Path, "/t/")
	note, _ := dal.GetNoteByName(t)
	if !note.Valid() {
		http.Redirect(w, r.Request, "/manage?q="+url.QueryEscape(t), 302)
		return
	}

	var notes []*types.Note
	if len(note.ParentIds) > 0 {
		notes, _ = dal.BatchGetNotes(note.ParentIds)
	}

	p, st, desc, pageSize := r.GetPagingArgs()

	results, total, pages := dal.KSVPaging(nil, fmt.Sprintf("children_%d", note.Id), st, desc, p-1, pageSize)
	tags, _ := dal.BatchGetNotes(results)

	r.AddTemplateValue("pid", "")
	r.AddTemplateValue("view", true)
	r.AddTemplateValue("note", note)
	r.AddTemplateValue("parents", notes)
	r.AddTemplateValue("tags", tags)
	r.AddTemplateValue("total", total)
	r.AddTemplateValue("pages", pages)

	httpTemplates.ExecuteTemplate(w, "manage.html", r)
}
