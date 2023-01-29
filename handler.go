package main

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strconv"
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
	if ok, remains := limiter.CheckIP(r); !ok {
		if remains == -1 {
			writeJSON(w, "success", false, "code", "IP_BANNED")
		} else {
			writeJSON(w, "success", false, "code", "COOLDOWN", "remains", remains)
		}
		return
	}

	if err := r.ParseMultipartForm(types.Config.RequestMaxSize); err != nil {
		if err == limiter.ErrRequestTooLarge {
			writeJSON(w, "success", false, "code", "CONTENT_TOO_LARGE")
		} else {
			logrus.Errorf("failed to parse multipart: %v", err)
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		}
		return
	}
	action := r.Header.Get("X-Ns-Action")

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

		if action == "touch" {
			if r.UserDisplay == target.Creator {
				writeJSON(w, "success", false, "code", "CANT_TOUCH_SELF")
				return
			}
			if err := dal.Store.Update(func(tx *bbolt.Tx) error {
				if dal.KSVExist(tx, "creator_"+r.UserDisplay, types.Uint64Bytes(target.Id)) {
					dal.DeleteCreator(tx, r.UserDisplay, target)
					target.TouchCount--
				} else {
					dal.UpdateCreator(tx, r.UserDisplay, target)
					target.TouchCount++
				}
				return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
			}); err != nil {
				writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
				return
			}
			ok = true
		} else if action == "approve" {
			ok = doApprove(target, false, w, r)
		} else if action == "reject" {
			ok = doReject(target, w, r)
		} else if action == "delete" || action == "Lock" || action == "Unlock" {
			if !r.User.IsMod() {
				writeJSON(w, "success", false, "code", "MODS_REQUIRED")
				return
			}
			if err := dal.Store.Update(func(tx *bbolt.Tx) error {
				if action == "delete" {
					dal.ProcessParentChanges(tx, target, target.ParentIds, nil)
					dal.DeleteCreator(tx, target.Creator, target)
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
		if action != "touch" {
			go dal.AppendHistory(target.Id, r.UserDisplay, action,
				types.UTF16Trunc(r.Form.Get("reject_msg"), 100),
				r.RemoteIPv4Masked(), target)
			limiter.AddIP(r, 1)
		} else {
			limiter.AddIP(r, 5)
		}
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

	if len(ad.hash) > 0 {
		dal.Store.Saver().AddAsync(bitmap.Uint64Key(id),
			types.DedupUint64(append(ad.hash, ngram.StrHash(r.UserDisplay))))
	}
	time.AfterFunc(time.Second*10, func() { dal.UploadS3(target.Image, imageThumbName(target.Image)) })
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
		writeJSON(w, "success", false, "code", "PENDING_REVIEW")
		return nil, false
	}

	if target.Lock && !r.User.IsMod() {
		writeJSON(w, "success", false, "code", "LOCKED")
		return nil, false
	}

	if target.Creator == "bulk" {
		target.Creator = r.UserDisplay
	}
	target.PendingReview = true
	target.ReviewTitle = ad.title
	target.ReviewContent = ad.content
	target.ReviewParentIds = ad.parentIds
	if ad.imageChanged {
		target.ReviewImage = ad.image
	} else {
		target.ReviewImage = target.Image
	}
	target.Modifier = r.UserDisplay
	target.UpdateUnix = clock.UnixMilli()

	if target.ReviewDataNotChanged() {
		writeJSON(w, "success", false, "code", "DATA_NO_CHANGE")
		return nil, false
	}

	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		dal.UpdateCreator(tx, target.Creator, target)
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	}); err != nil {
		logrus.Errorf("update %d: %v", id, err)
		writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		return nil, false
	}

	if r.User.IsMod() || r.UserDisplay == target.Creator {
		// We don't want to alter 'target', so pass a copy into doApprove.
		if !doApprove(target.Clone(), true, w, r) {
			return nil, false
		}
	}

	time.AfterFunc(time.Second*10, func() { dal.UploadS3(ad.image, imageThumbName(ad.image)) })
	return target, true
}

func doApprove(target *types.Note, direct bool, w http.ResponseWriter, r *types.Request) bool {
	idKey := types.Uint64Bytes(target.Id)

	defer func(start time.Time) {
		logrus.Infof("approve %d in %v, direct update: %v", target.Id, time.Since(start), direct)
	}(time.Now())

	if !target.PendingReview {
		writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		return false
	}

	if !r.User.IsMod() && target.Creator != r.UserDisplay {
		writeJSON(w, "success", false, "code", "ILLEGAL_APPROVE")
		return false
	}

	var exist bool
	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		if tt := target.ReviewTitle; tt != "" {
			if key, found := dal.KSVFirstKeyOfSort1(tx, dal.NoteBK, []byte(tt)); found {
				if !bytes.Equal(key, idKey[:]) {
					exist = true
					return nil
				}
			}
		}

		dal.ProcessParentChanges(tx, target, target.ParentIds, target.ReviewParentIds)

		target.Title = target.ReviewTitle
		target.Content = target.ReviewContent
		target.Image = target.ReviewImage
		target.ParentIds = target.ReviewParentIds
		target.Reviewer = r.UserDisplay
		target.ClearReviewStatus()
		dal.UpdateCreator(tx, target.Creator, target)
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
	h := buildBitmapHashes(target.Title, target.Creator, target.ParentIds)
	dal.Store.Saver().AddAsync(bitmap.Uint64Key(target.Id), types.DedupUint64(h))
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
		dal.UpdateCreator(tx, target.Creator, target)
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	}); err != nil {
		logrus.Errorf("reject %d: %v", target.Id, err)
		writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
		return false
	}
	return true
}

func HandleHistory(w http.ResponseWriter, r *types.Request) {
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
		if note, _ = dal.GetNote(id); note.Valid() {
			results, total, pages = dal.KSVPaging(nil, fmt.Sprintf("history_%d", note.Id), -1, r.P.Desc, r.P.Page-1, r.P.PageSize)
		} else {
			http.Redirect(w, r.Request, "/ns:not_found", 302)
			return
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

func HandleTagSearch(w http.ResponseWriter, r *types.Request) {
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
		ids, jms = collectSimple(expandQuery(q))
	}

	tags, _ := dal.BatchGetNotes(ids)
	sort.Slice(tags, func(i, j int) bool { return len(tags[i].Title) < len(tags[j].Title) })

	results := []interface{}{}
	doubleCheckFilterResults(q, nil, tags, func(t *types.Note) bool {
		tt := t.Title
		if tt == "" {
			tt = "ns:id:" + strconv.FormatUint(t.Id, 10)
		}
		results = append(results, [3]interface{}{t.Id, tt, t.ChildrenCount})
		return len(results) < n
	})
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
		http.Redirect(w, r.Request, "/ns:not_found", 302)
		return
	}
	r.AddTemplateValue("note", note)
	r.AddTemplateValue("readonly", readonly)
	r.AddTemplateValue("recordUnix", recordUnix)
	httpTemplates.ExecuteTemplate(w, "edit.html", r)
}

func HandleView(t string, w http.ResponseWriter, r *types.Request) {
	note, _ := dal.GetNoteByName(t)
	if !note.Valid() {
		http.Redirect(w, r.Request, "/ns:manage?q="+types.FullEscape(t), 302)
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

func HandleManage(w http.ResponseWriter, r *types.Request) {
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
			ids, _ := collectSimple(query, pids, uid)
			res, _ := dal.BatchGetNotes(ids)
			total = len(res)
			res = sortNotes(res, r.P.Sort, r.P.Desc)
			doubleCheckFilterResults(query, pids, res, func(t *types.Note) bool {
				notes = append(notes, t)
				return len(notes) < 500
			})
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
