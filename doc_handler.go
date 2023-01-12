package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
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

//go:embed static/assets/*
var httpStaticAssets embed.FS

func HandleAssets(w http.ResponseWriter, r *types.Request) {
	p := "static/assets/" + strings.TrimPrefix(r.URL.Path, "/assets/")
	buf, _ := httpStaticAssets.ReadFile(p)
	w.Write(buf)
}

func HandlePostPage(w http.ResponseWriter, r *types.Request) {
	httpTemplates.ExecuteTemplate(w, "post.html", r)
}

func HandleTagAction(w http.ResponseWriter, r *types.Request) {
	q := r.URL.Query()
	id, err := strconv.ParseUint(q.Get("id"), 10, 64)
	action := q.Get("action")
	idKey := bitmap.Uint64Key(id)

	var target *types.Tag
	if action != "create" {
		dal.LockKey(id)
		defer dal.UnlockKey(id)
		target, err = dal.GetTag(id)
		if !target.Valid() || err != nil {
			logrus.Errorf("tag manage action %s, can't find %d: %v", action, id, err)
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
			return
		}
	}

	switch action {
	case "Lock", "Unlock" /* "approve", "reject", */, "delete":
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
		n, content := strings.TrimSpace(q.Get("title")), strings.TrimSpace(q.Get("content"))
		h := buildBitmapHashes(n)
		if len(n) < 1 || utf16Len(n) > 50 || len(h) == 0 || utf16Len(content) > 50000 {
			writeJSON(w, "success", false, "code", "INVALID_CONTENT")
			return
		}
		if action == "create" && strings.HasPrefix(n, "ts:") && !r.User.IsMod() {
			writeJSON(w, "success", false, "code", "MODS_REQUIRED")
			return
		}

		var parentTags []uint64
		if pt := q.Get("parents"); pt != "" {
			gjson.Parse(pt).ForEach(func(key, value gjson.Result) bool {
				parentTags = append(parentTags, key.Uint())
				return true
			})
			if len(parentTags) > 8 {
				writeJSON(w, "success", false, "code", "TOO_MANY_PARENTS")
				return
			}
		}

		var err error
		var exist, addDirectly bool
		if action == "create" {
			target = &types.Tag{
				Id:        clock.Id(),
				Name:      n,
				Content:   content,
				Creator:   r.UserDisplay,
				Modifier:  r.UserDisplay,
				ParentIds: parentTags,
			}
			idKey = bitmap.Uint64Key(target.Id)
			exist, err = dal.CreateTag(n, target)
			addDirectly = true
		} else {
			err = dal.TagsStore.Update(func(tx *bbolt.Tx) error {
				if n != target.Name {
					if _, exist = dal.KSVFirstKeyOfSort1(tx, "tags", []byte(n)); exist {
						return nil
					}
				}
				dal.ProcessTagParentChanges(tx, target, target.ParentIds, parentTags)
				target.ParentIds = parentTags
				if r.User.IsMod() || r.UserDisplay == target.Creator {
					target.Name = n
					target.Content = content
					addDirectly = true
				} else {
					target.PendingReview = true
					target.ReviewName = n
					target.ReviewDesc = content
				}
				target.Modifier = r.UserDisplay
				target.UpdateUnix = clock.UnixMilli()
				dal.UpdateTagCreator(tx, target)
				return dal.KSVUpsert(tx, "tags", dal.KSVFromTag(target))
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
		if addDirectly {
			dal.TagsStore.Saver().AddAsync(idKey, h)
		}
	case "delete":
		if err := dal.TagsStore.Update(func(tx *bbolt.Tx) error {
			dal.ProcessTagParentChanges(tx, target, target.ParentIds, nil)
			return dal.KSVDelete(tx, "tags", idKey[:])
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
		if action == "approve" {
			if !r.User.IsMod() && target.Creator != r.UserDisplay {
				writeJSON(w, "success", false, "code", "ILLEGAL_APPROVE")
				return
			}
			target.Name = target.ReviewName
			target.Content = target.ReviewDesc
			target.Reviewer = r.UserDisplay
		}
		target.PendingReview = false
		target.ReviewName, target.ReviewDesc = "", ""

		var exist bool
		if err := dal.TagsStore.Update(func(tx *bbolt.Tx) error {
			if target.Name == "" && action == "reject" {
				return dal.KSVDelete(tx, "tags", idKey[:])
			}
			if key, found := dal.KSVFirstKeyOfSort1(tx, "tags", []byte(target.Name)); found {
				if !bytes.Equal(key, idKey[:]) {
					exist = true
					return nil
				}
			}
			dal.UpdateTagCreator(tx, target)
			return dal.KSVUpsert(tx, "tags", dal.KSVFromTag(target))
		}); err != nil {
			logrus.Errorf("tag manage action %s %d: %v", action, id, err)
			writeJSON(w, "success", false, "code", "INTERNAL_ERROR")
			return
		}
		if exist {
			writeJSON(w, "success", false, "code", "DUPLICATED_TITLE")
			return
		}
		dal.TagsStore.Saver().AddAsync(idKey, buildBitmapHashes(target.Name))
	case "Lock", "Unlock":
		target.Lock = action == "lock"
		target.UpdateUnix = clock.UnixMilli()
		if err := dal.TagsStore.Update(func(tx *bbolt.Tx) error {
			return dal.KSVUpsert(tx, "tags", dal.KSVFromTag(target))
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
	writeJSON(w, "success", true, "tag", target)
}

func HandleTagManage(w http.ResponseWriter, r *types.Request) {
	p, st, desc, pageSize := r.GetPagingArgs()
	q := r.URL.Query().Get("q")
	pidStr := r.URL.Query().Get("pid")

	var tags []*types.Tag
	var total, pages int
	if q != "" {
		st, desc = -1, false
		ids, _ := collectSimple(q)
		tags, _ = dal.BatchGetTags(ids)
		total = len(tags)
		sort.Slice(tags, func(i, j int) bool { return len(tags[i].Name) < len(tags[j].Name) })
		tags = tags[:imin(500, len(tags))]
	} else {
		var results []dal.KeySortValue
		if !strings.HasPrefix(pidStr, "@") {
			pid, _ := strconv.ParseUint(pidStr, 10, 64)
			if pid > 0 {
				results, total, pages = dal.KSVPaging(nil, fmt.Sprintf("tags_children_%d", pid), st, desc, p-1, pageSize)
				ids := make([]bitmap.Key, len(results))
				for i := range ids {
					ids[i] = bitmap.BytesKey(results[i].Key)
				}
				tags, _ = dal.BatchGetTags(ids)
				ptag, _ := dal.GetTag(pid)
				r.AddTemplateValue("ptag", ptag)
			} else {
				results, total, pages = dal.KSVPaging(nil, "tags", st, desc, p-1, pageSize)
				tags = make([]*types.Tag, len(results))
				for i := range tags {
					tags[i] = types.UnmarshalTagBinary(results[i].Value)
				}
				pidStr = ""
			}
		} else {
			results, total, pages = dal.KSVPaging(nil, "tags_creator_"+pidStr[1:], st, desc, p-1, pageSize)
			tags, _ = dal.BatchGetTags(results)
		}
	}

	if editTagID, _ := strconv.Atoi(r.URL.Query().Get("edittagid")); editTagID > 0 {
		found := false
		for _, t := range tags {
			found = found || t.Id == uint64(editTagID)
		}
		if !found {
			if tag, _ := dal.GetTag(uint64(editTagID)); tag.Valid() {
				tags = append(tags, tag)
			}
		}
	}

	r.AddTemplateValue("query", q)
	r.AddTemplateValue("pid", pidStr)
	r.AddTemplateValue("pid_is_user", strings.HasPrefix(pidStr, "@"))
	r.AddTemplateValue("tags", tags)
	r.AddTemplateValue("total", total)
	r.AddTemplateValue("pages", pages)
	r.AddTemplateValue("page", p)
	r.AddTemplateValue("sort", st)
	r.AddTemplateValue("desc", desc)
	httpTemplates.ExecuteTemplate(w, "manage.html", r)
}

func HandleHistory(w http.ResponseWriter, r *types.Request) {
	p, _, desc, pageSize := r.GetPagingArgs()
	idStr := r.URL.Query().Get("id")
	if idStr == "0" {
		idStr = ""
	}
	var results []dal.KeySortValue
	var records []*types.TagRecord
	var tag *types.Tag
	var total, pages int

	if idStr == "" {
		results, total, pages = dal.KSVPaging(nil, "tags_history", -1, desc, p-1, pageSize)
		tag = &types.Tag{}
	} else if !strings.HasPrefix(idStr, "@") {
		id, _ := strconv.ParseUint(idStr, 10, 64)
		if tag, _ = dal.GetTag(id); tag.Valid() {
			results, total, pages = dal.KSVPaging(nil, fmt.Sprintf("tags_history_%d", tag.Id), -1, desc, p-1, pageSize)
		}
	} else {
		results, total, pages = dal.KSVPaging(nil, "tags_history_"+idStr[1:], -1, desc, p-1, pageSize)
	}

	for i := range results {
		var t *types.TagRecord
		if idStr == "" {
			t = types.UnmarshalTagRecordBinary(results[i].Value)
		} else {
			t, _ = dal.GetTagRecord(bitmap.BytesKey(results[i].Key))
			if t == nil || t.Tag == nil {
				continue
			}
		}
		records = append(records, t)
	}

	r.AddTemplateValue("id", idStr)
	r.AddTemplateValue("tag", tag)
	r.AddTemplateValue("total", total)
	r.AddTemplateValue("records", records)
	r.AddTemplateValue("pages", pages)
	r.AddTemplateValue("page", p)
	r.AddTemplateValue("desc", desc)
	httpTemplates.ExecuteTemplate(w, "tag_history.html", r)
}

func HandleTagSearch(w http.ResponseWriter, r *types.Request) {
	start := time.Now()
	q := utf16Trunc(r.URL.Query().Get("q"), 50)
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n == 0 {
		n = 100
	}
	n = imin(100, n)
	n = imax(1, n)

	var ids []bitmap.Key
	var jms []bitmap.JoinMetrics

	if tagIDs := r.URL.Query().Get("ids"); tagIDs != "" {
		for _, p := range strings.Split(tagIDs, ",") {
			id, _ := strconv.ParseUint(p, 10, 64)
			ids = append(ids, bitmap.Uint64Key(id))
		}
	} else {
		ids, jms = collectSimple(q)
	}

	tags, _ := dal.BatchGetTags(ids)
	sort.Slice(tags, func(i, j int) bool { return len(tags[i].Name) < len(tags[j].Name) })

	results := []interface{}{}
	h := ngram.SplitMore(q)
	for i, tag := range tags {
		if i >= n {
			break
		}
		if tag.Name != "" {
			if len(h) == 0 || ngram.SplitMore(tag.Name).Contains(h) {
				results = append(results, [3]interface{}{
					tag.Id,
					tag.Name,
					tag.ParentIds,
				})
			}
		}
	}
	diff := time.Since(start)
	writeJSON(w,
		"success", true,
		"tags", results,
		"elapsed", diff.Milliseconds(),
		"elapsed_us", diff.Microseconds(),
		"debug", fmt.Sprint(jms),
		"count", len(results),
	)
}

func HandleTagNew(w http.ResponseWriter, r *types.Request) {
	httpTemplates.ExecuteTemplate(w, "tag_new.html", r)
}

func HandleSingleTag(w http.ResponseWriter, r *types.Request) {
	t := strings.TrimPrefix(r.URL.Path, "/t/")
	tag, _ := dal.GetTagByName(t)
	if tag.Valid() {
		http.Redirect(w, r.Request, "/manage?edittagid="+strconv.FormatUint(tag.Id, 10), 302)
	} else {
		http.Redirect(w, r.Request, "/manage?q="+url.QueryEscape(t), 302)
	}
}

func HandleTagStoreStatus(w http.ResponseWriter, r *types.Request) {
	stats := dal.TagsStore.DB.Stats()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(stats)
	w.Write([]byte("\n"))

	fi, err := os.Stat(dal.TagsStore.DB.Path())
	if err != nil {
		fmt.Fprintf(w, "<failed to read data on disk>\n\n")
	} else {
		sz := fi.Size()
		fmt.Fprintf(w, "data on disk: %d (%.2f)\n\n", sz, float64(sz)/1024/1024)
	}

	dal.TagsStore.WalkDesc(clock.UnixMilli(), func(b *bitmap.Range) bool {
		fmt.Fprint(w, b.String())
		return true
	})
}
