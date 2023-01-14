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
	"github.com/coyove/iis/limiter"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/coyove/sdss/contrib/ngram"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

func HandleTagAction(w http.ResponseWriter, r *types.Request) {
	if !dal.CheckIP(r.RemoteIPv4) {
		writeJSON(w, "success", false, "code", "IP_BANNED")
		return
	}

	if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
		if err == limiter.ErrRequestTooLarge {
			writeJSON(w, "success", false, "code", "CONTENT_TOO_LARGE")
		} else {
			logrus.Errorf("failed to parse multipart: %v", err)
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		}
		return
	}
	action := r.Form.Get("action")

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
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
			return
		}

		if action == "approve" {
			ok = doApprove(target, w, r)
		} else if action == "reject" {
			ok = doReject(target, w, r)
		} else if action == "delete" || action == "Lock" || action == "Unlock" {
			if !r.User.IsMod() {
				writeJSON(w, "success", false, "code", "MODS_REQUIRED")
				return
			}
			if err := dal.Store.Update(func(tx *bbolt.Tx) error {
				if action == "delete" {
					dal.ProcessTagParentChanges(tx, target, target.ParentIds, nil)
					return dal.KSVDelete(tx, dal.NoteBK, types.Uint64Bytes(target.Id))
				}
				target.Lock = action == "Lock"
				target.UpdateUnix = clock.UnixMilli()
				return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
			}); err != nil {
				logrus.Errorf("mod %s %d: %v", action, id, err)
				writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
				return
			}
			ok = true
		} else {
			writeJSON(w, "success", false, "code", "INVALID_ACTION")
			return
		}
	}
	if ok {
		go dal.ProcessTagHistory(target.Id, r.UserDisplay, action, r.RemoteIPv4Masked(), target)
		writeJSON(w, "success", true, "note", target)
	}
}

func doCreate(w http.ResponseWriter, r *types.Request) (*types.Note, bool) {
	id := clock.Id()
	ad, msg := getActionData(id, r)

	defer func(start time.Time) {
		logrus.Infof("create %d, data: %v in %v", id, ad, time.Since(start))
	}(time.Now())

	if msg != "" {
		writeJSON(w, "success", false, "code", msg)
		return nil, false
	}

	target := &types.Note{
		Id:        id,
		Title:     ad.title,
		Content:   ad.content,
		Image:     ad.image,
		Creator:   r.UserDisplay,
		ParentIds: ad.parentIds,
	}
	exist, err := dal.CreateNote(ad.title, target)
	if err != nil {
		logrus.Errorf("create %d: %v", id, err)
		writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		return nil, false
	}
	if exist {
		writeJSON(w, "success", false, "code", "DUPLICATED_TITLE")
		return nil, false
	}

	dal.Store.Saver().AddAsync(bitmap.Uint64Key(id), ad.hash)
	return target, true
}

func doUpdate(w http.ResponseWriter, r *types.Request) (*types.Note, bool) {
	id, _ := strconv.ParseUint(r.Form.Get("id"), 10, 64)

	ad, msg := getActionData(id, r)
	defer func(start time.Time) {
		logrus.Infof("update %d, data: %v in %v", id, ad, time.Since(start))
	}(time.Now())

	if msg != "" {
		writeJSON(w, "success", false, "code", msg)
		return nil, false
	}

	dal.LockKey(id)
	defer dal.UnlockKey(id)

	target, err := dal.GetNote(id)
	if !target.Valid() || err != nil {
		logrus.Errorf("update can't load note %d: %v", id, err)
		writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		return nil, false
	}

	if target.PendingReview {
		if target.Modifier == r.UserDisplay || target.Creator == r.UserDisplay || r.User.IsMod() {
			// Creator, modifier and moderators can still update the content.
		} else {
			writeJSON(w, "success", false, "code", "PENDING_REVIEW")
			return nil, false
		}
	}

	if target.Lock && !r.User.IsMod() {
		writeJSON(w, "success", false, "code", "LOCKED")
		return nil, false
	}

	var exist, shouldIndex bool
	err = dal.Store.Update(func(tx *bbolt.Tx) error {
		if ad.title != target.Title {
			if _, exist = dal.KSVFirstKeyOfSort1(tx, dal.NoteBK, []byte(ad.title)); exist {
				return nil
			}
		}
		dal.ProcessTagParentChanges(tx, target, target.ParentIds, ad.parentIds)
		target.ParentIds = ad.parentIds
		if target.Creator == "bulk" {
			target.Creator = r.UserDisplay
		}
		if r.User.IsMod() || r.UserDisplay == target.Creator {
			shouldIndex = ad.title != target.Title
			target.Title = ad.title
			target.Content = ad.content
			if ad.imageChanged {
				target.Image = ad.image
			}
			target.ClearReviewStatus()
		} else {
			target.PendingReview = true
			target.ReviewTitle = ad.title
			target.ReviewContent = ad.content
			if ad.imageChanged {
				target.ReviewImage = ad.image
			} else {
				target.ReviewImage = target.Image
			}
		}
		target.Modifier = r.UserDisplay
		target.UpdateUnix = clock.UnixMilli()
		dal.UpdateCreator(tx, target)
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	})
	if err != nil {
		logrus.Errorf("update %d: %v", id, err)
		writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		return nil, false
	}
	if exist {
		writeJSON(w, "success", false, "code", "DUPLICATED_TITLE")
		return nil, false
	}
	if shouldIndex {
		dal.Store.Saver().AddAsync(bitmap.Uint64Key(id), ad.hash)
	}
	return target, true
}

func doApprove(target *types.Note, w http.ResponseWriter, r *types.Request) bool {
	idKey := types.Uint64Bytes(target.Id)

	defer func(start time.Time) {
		logrus.Infof("approve %d in %v", target.Id, time.Since(start))
	}(time.Now())

	if !target.PendingReview {
		writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		return false
	}

	if !r.User.IsMod() && target.Creator != r.UserDisplay {
		writeJSON(w, "success", false, "code", "ILLEGAL_APPROVE")
		return false
	}

	shouldIndex := target.Title != target.ReviewTitle
	target.Title = target.ReviewTitle
	target.Content = target.ReviewContent
	target.Image = target.ReviewImage
	target.Reviewer = r.UserDisplay
	target.ClearReviewStatus()

	var exist bool
	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		if key, found := dal.KSVFirstKeyOfSort1(tx, dal.NoteBK, []byte(target.Title)); found {
			if !bytes.Equal(key, idKey[:]) {
				exist = true
				return nil
			}
		}
		dal.UpdateCreator(tx, target)
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	}); err != nil {
		logrus.Errorf("approve %d: %v", target.Id, err)
		writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		return false
	}
	if exist {
		writeJSON(w, "success", false, "code", "DUPLICATED_TITLE")
		return false
	}
	if shouldIndex {
		dal.Store.Saver().AddAsync(bitmap.Uint64Key(target.Id), buildBitmapHashes(target.Title))
	}
	return true
}

func doReject(target *types.Note, w http.ResponseWriter, r *types.Request) bool {
	if !target.PendingReview {
		writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		return false
	}
	if !r.User.IsMod() && target.Creator != r.UserDisplay && target.Modifier != r.UserDisplay {
		writeJSON(w, "success", false, "code", "ILLEGAL_APPROVE")
		return false
	}
	target.ClearReviewStatus()

	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		if target.Title == "" {
			return dal.KSVDelete(tx, dal.NoteBK, types.Uint64Bytes(target.Id))
		}
		dal.UpdateCreator(tx, target)
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	}); err != nil {
		logrus.Errorf("reject %d: %v", target.Id, err)
		writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		return false
	}
	return true
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
			http.Redirect(w, r.Request, "/ns:notfound", 302)
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

func HandleNew(w http.ResponseWriter, r *types.Request) {
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
		http.Redirect(w, r.Request, "/ns:notfound", 302)
		return
	}
	r.AddTemplateValue("note", note)
	r.AddTemplateValue("readonly", readonly)
	r.AddTemplateValue("recordUnix", recordUnix)
	httpTemplates.ExecuteTemplate(w, "edit.html", r)
}

func HandleView(w http.ResponseWriter, r *types.Request) {
	t := strings.TrimPrefix(r.URL.Path, "/")
	note, _ := dal.GetNoteByName(t)
	if !note.Valid() {
		http.Redirect(w, r.Request, "/ns:manage?q="+url.QueryEscape(t), 302)
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
