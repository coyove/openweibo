package main

import (
	"bytes"
	"strconv"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/coyove/sdss/contrib/ngram"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

func doCreate(w *types.Response, r *types.Request) (*types.Note, bool) {
	id := clock.Id()
	ad, msg := getActionData(id, r)

	defer func(start time.Time) {
		logrus.Infof("create %d, data: %v in %v", id, ad, time.Since(start))
	}(time.Now())

	if msg != "" {
		w.WriteJSON("success", false, "code", msg)
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
		w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
		return nil, false
	}
	if exist {
		w.WriteJSON("success", false, "code", "DUPLICATED_TITLE")
		return nil, false
	}

	if len(ad.hash) > 0 {
		dal.Store.Saver().AddAsync(bitmap.Uint64Key(id),
			types.DedupUint64(append(ad.hash, ngram.StrHash(r.UserDisplay))))
	}
	time.AfterFunc(time.Second*10, func() { dal.UploadS3(target.Image, imageThumbName(target.Image)) })
	return target, true
}

func doUpdate(w *types.Response, r *types.Request) (*types.Note, bool) {
	id, _ := strconv.ParseUint(r.Form.Get("id"), 10, 64)

	ad, msg := getActionData(id, r)
	defer func(start time.Time) {
		logrus.Infof("update %d, data: %v in %v", id, ad, time.Since(start))
	}(time.Now())

	if msg != "" {
		w.WriteJSON("success", false, "code", msg)
		return nil, false
	}

	dal.LockKey(id)
	defer dal.UnlockKey(id)

	target, err := dal.GetNote(id)
	if !target.Valid() || err != nil {
		logrus.Errorf("update can't load note %d: %v", id, err)
		w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
		return nil, false
	}

	if target.PendingReview {
		w.WriteJSON("success", false, "code", "PENDING_REVIEW")
		return nil, false
	}

	if target.Lock && !r.User.IsMod() {
		w.WriteJSON("success", false, "code", "LOCKED")
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
		w.WriteJSON("success", false, "code", "DATA_NO_CHANGE")
		return nil, false
	}

	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		dal.UpdateCreator(tx, target.Creator, target)
		dal.AppendImage(tx, target)
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	}); err != nil {
		logrus.Errorf("update %d: %v", id, err)
		w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
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

func doApprove(target *types.Note, direct bool, w *types.Response, r *types.Request) bool {
	idKey := types.Uint64Bytes(target.Id)

	defer func(start time.Time) {
		logrus.Infof("approve %d in %v, direct update: %v", target.Id, time.Since(start), direct)
	}(time.Now())

	if !target.PendingReview {
		w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
		return false
	}

	if !r.User.IsMod() && target.Creator != r.UserDisplay {
		w.WriteJSON("success", false, "code", "ILLEGAL_APPROVE")
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
		w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
		return false
	}
	if exist {
		w.WriteJSON("success", false, "code", "DUPLICATED_TITLE")
		return false
	}
	h := buildBitmapHashes(target.Title, target.Creator, target.ParentIds)
	dal.Store.Saver().AddAsync(bitmap.Uint64Key(target.Id), types.DedupUint64(h))
	return true
}

func doReject(target *types.Note, w *types.Response, r *types.Request) bool {
	if !target.PendingReview {
		w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
		return false
	}
	if !r.User.IsMod() && target.Creator != r.UserDisplay && target.Modifier != r.UserDisplay {
		w.WriteJSON("success", false, "code", "ILLEGAL_APPROVE")
		return false
	}
	target.ClearReviewStatus()

	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	}); err != nil {
		logrus.Errorf("reject %d: %v", target.Id, err)
		w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
		return false
	}
	return true
}

func doTouch(target *types.Note, w *types.Response, r *types.Request) bool {
	if r.UserDisplay == target.Creator {
		w.WriteJSON("success", false, "code", "CANT_TOUCH_SELF")
		return false
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
		w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
		return false
	}
	return true
}

func doDelete(target *types.Note, w *types.Response, r *types.Request) bool {
	if !r.User.IsMod() {
		if r.UserDisplay != target.Creator {
			w.WriteJSON("success", false, "code", "MODS_REQUIRED")
			return false
		}
		if clock.UnixMilli()-target.CreateUnix > 300*1000 {
			w.WriteJSON("success", false, "code", "DELETE_CHANCE_EXPIRED")
			return false
		}
	}
	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		dal.ProcessParentChanges(tx, target, target.ParentIds, nil)
		dal.DeleteCreator(tx, target.Creator, target)
		return dal.KSVDelete(tx, dal.NoteBK, types.Uint64Bytes(target.Id))
	}); err != nil {
		logrus.Errorf("delete %d: %v", target.Id, err)
		w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
		return false
	}
	go dal.DeleteS3(dal.ListImage(target.Id, true)...)
	return true
}

func doLockUnlock(target *types.Note, lock bool, w *types.Response, r *types.Request) bool {
	if !r.User.IsMod() {
		w.WriteJSON("success", false, "code", "MODS_REQUIRED")
		return false
	}
	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		target.Lock = lock
		target.UpdateUnix = clock.UnixMilli()
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	}); err != nil {
		logrus.Errorf("lock(%v) %d: %v", lock, target.Id, err)
		w.WriteJSON("success", false, "code", "INTERNAL_ERROR")
		return false
	}
	return true
}

func doPreview(w *types.Response, r *types.Request) {
	css, _ := httpStaticAssets.ReadFile("static/assets/main.css")
	out := &bytes.Buffer{}
	out.WriteString("<!doctype html><html><meta charset='UTF-8'><style>")
	out.Write(css)
	out.WriteString("</style><div id=container><pre class=note>")
	out.WriteString(types.RenderClip(r.Form.Get("content")))
	out.WriteString("</pre></div></html>")
	w.WriteJSON("success", true, "content", out.String())
}
