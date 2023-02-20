package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"math/rand"
	"time"
	"unicode"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/coyove/sdss/contrib/ngram"
	"github.com/coyove/sdss/contrib/simple"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
)

type writeError string

func (e writeError) Error() string { return string(e) }

func doCreate(id uint64, r *types.Request) error {
	ad, msg := getActionData(id, r)
	if msg != "" {
		return writeError(msg)
	}

	target := &types.Note{
		Id:        id,
		Title:     ad.title,
		Content:   ad.content,
		Image:     ad.image,
		Creator:   r.UserDisplay,
		ParentIds: ad.parentIds,
	}
	exist, err := dal.CreateNote(ad.title, target, ad.imageTotal > 1)
	if err != nil {
		return writeError("INTERNAL_ERROR")
	}
	if exist {
		return writeError("DUPLICATED_TITLE")
	}

	if len(ad.hash) > 0 {
		dal.Store.Saver().AddAsync(bitmap.Uint64Key(id),
			simple.Uint64.Dedup(append(ad.hash, ngram.StrHash(r.UserDisplay))))
	}

	time.AfterFunc(time.Second*10, func() { dal.UploadS3(target.Image, imageThumbName(target.Image)) })
	logrus.Infof("create %d, data: %v", id, ad)
	return nil
}

func doUpdate(id uint64, r *types.Request) error {
	ad, msg := getActionData(id, r)
	if msg != "" {
		return writeError(msg)
	}

	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		target := dal.GetNoteTx(tx, id)
		if !target.Valid() {
			return writeError("ID_NOT_FOUND")
		}

		if target.PendingReview {
			if !r.User.IsMod() && r.UserDisplay != target.Modifier && r.UserDisplay != target.Creator {
				return writeError("PENDING_REVIEW")
			}
		}

		if target.Lock && !r.User.IsMod() {
			return writeError("LOCKED")
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
			return writeError("DATA_NO_CHANGE")
		}

		dal.UpdateCreator(tx, target.Creator, target)
		dal.AppendImage(tx, target)

		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromNote(target))
	}); err != nil {
		return err
	}

	time.AfterFunc(time.Second*10, func() { dal.UploadS3(ad.image, imageThumbName(ad.image)) })
	logrus.Infof("update %d, data: %v", id, ad)
	return nil
}

func doApprove(id uint64, r *types.Request) error {
	var target *types.Note
	var changed bool
	err := dal.Store.Update(func(tx *bbolt.Tx) error {
		target = dal.GetNoteTx(tx, id)
		if !target.Valid() {
			return writeError("ID_NOT_FOUND")
		}
		if !target.PendingReview {
			return writeError("INTERNAL_ERROR")
		}
		if !r.User.IsMod() && target.Creator != r.UserDisplay {
			return writeError("ILLEGAL_APPROVE")
		}
		if tt := target.ReviewTitle; tt != "" {
			if key, found := dal.KSVFirstKeyOfSort1(tx, dal.NoteBK, []byte(tt)); found {
				if !bytes.Equal(key, types.Uint64Bytes(target.Id)) {
					return writeError("DUPLICATED_TITLE")
				}
			}
		}

		changed = target.ReviewTitle != target.Title || !simple.Uint64.Equal(target.ParentIds, target.ReviewParentIds)
		dal.ProcessParentChanges(tx, target, target.ParentIds, target.ReviewParentIds)

		target.Title = target.ReviewTitle
		target.Content = target.ReviewContent
		target.Image = target.ReviewImage
		target.ParentIds = target.ReviewParentIds
		target.Reviewer = r.UserDisplay
		target.ClearReviewStatus()
		dal.UpdateCreator(tx, target.Creator, target)

		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromNote(target))
	})
	if err == nil {
		if changed {
			h := buildBitmapHashes(target.Title, target.Creator, target.ParentIds)
			dal.Store.Saver().AddAsync(bitmap.Uint64Key(target.Id), simple.Uint64.Dedup(h))
		}
	}
	return err
}

func doReject(id uint64, r *types.Request) error {
	return dal.Store.Update(func(tx *bbolt.Tx) error {
		target := dal.GetNoteTx(tx, id)
		if !target.Valid() {
			return writeError("ID_NOT_FOUND")
		}
		if !target.PendingReview {
			return writeError("INTERNAL_ERROR")
		}
		if !r.User.IsMod() && target.Creator != r.UserDisplay && target.Modifier != r.UserDisplay {
			return writeError("ILLEGAL_APPROVE")
		}
		target.ClearReviewStatus()
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromNote(target))
	})
}

func doTouch(id uint64, r *types.Request) error {
	return dal.Store.Update(func(tx *bbolt.Tx) error {
		target := dal.GetNoteTx(tx, id)
		if !target.Valid() {
			return writeError("ID_NOT_FOUND")
		}
		if r.UserDisplay == target.Creator {
			return writeError("CANT_TOUCH_SELF")
		}
		if dal.KSVExist(tx, "creator_"+r.UserDisplay, types.Uint64Bytes(target.Id)) {
			dal.DeleteCreator(tx, r.UserDisplay, target)
			target.TouchCount--
		} else {
			dal.UpdateCreator(tx, r.UserDisplay, target)
			target.TouchCount++
		}
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromNote(target))
	})
}

func doDeleteHide(hide bool, r *types.Request) error {
	ids := simple.Uint64.Dedup(types.SplitUint64List(r.Form.Get("ids")))

	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		for _, id := range ids {
			n := dal.GetNoteTx(tx, id)
			if !n.Valid() {
				continue
			}
			if !r.User.IsMod() && r.UserDisplay != n.Creator {
				return writeError("MODS_REQUIRED")
			}
			if hide {
				if err := dal.KSVDeleteSort0(tx, dal.NoteBK, types.Uint64Bytes(n.Id)); err != nil {
					return err
				}
			} else {
				dal.ProcessParentChanges(tx, n, n.ParentIds, nil)
				dal.DeleteCreator(tx, n.Creator, n)
				if err := dal.KSVDelete(tx, dal.NoteBK, types.Uint64Bytes(n.Id)); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if !hide {
		go func() {
			for _, id := range ids {
				var imgs []string
				for _, img := range dal.ListImage(id, true) {
					imgs = append(imgs, img, imageThumbName(img))
				}
				dal.DeleteS3(imgs...)
			}
		}()
	}
	logrus.Infof("delete(%v) data: %v", hide, ids)
	return nil
}

func doLockUnlock(id uint64, lock bool, r *types.Request) error {
	if !r.User.IsMod() {
		return writeError("MODS_REQUIRED")
	}
	return dal.Store.Update(func(tx *bbolt.Tx) error {
		target := dal.GetNoteTx(tx, id)
		if !target.Valid() {
			return writeError("ID_NOT_FOUND")
		}
		target.Lock = lock
		target.UpdateUnix = clock.UnixMilli()
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromNote(target))
	})
}

func doPreview(w *types.Response, r *types.Request) {
	out := &bytes.Buffer{}
	out.WriteString("<pre class=note>")
	out.WriteString(types.RenderClip(r.Form.Get("content")))
	out.WriteString("</pre>")
	w.WriteJSON("success", true, "content", out.String())
}

func doUserLogin(register bool, r *types.Request) (*types.User, error) {
	id := r.Form.Get("id")
	p := types.UTF16Trunc(r.Form.Get("password"), 100)
	email := types.UTF16Trunc(r.Form.Get("email"), 100)
	if id == "" || p == "" {
		return nil, writeError("INVALID_REGISTER_INFO")
	}
	if register && email == "" {
		return nil, writeError("INVALID_REGISTER_INFO")
	}
	if types.UTF16LenExceeds(id, 20) {
		return nil, writeError("INVALID_REGISTER_INFO")
	}
	for _, r := range id {
		if unicode.IsSpace(r) || r == '<' || r == '>' || r == '&' || r == ':' || r == '"' || r == '\'' {
			return nil, writeError("INVALID_REGISTER_ID")
		}
	}

	h := hmac.New(sha1.New, []byte(types.Config.Key))
	h.Write([]byte(p))
	pwd := h.Sum(nil)

	target := &types.User{
		Id:         id,
		PwdHash:    pwd,
		Email:      email,
		CreateUnix: clock.UnixMilli(),
		LoginUnix:  clock.UnixMilli(),
		CreateUA:   r.UserAgent(),
		CreateIP:   r.RemoteIPv4.String(),
		LoginUA:    r.UserAgent(),
		LoginIP:    r.RemoteIPv4.String(),
		Session64:  rand.Int63(),
	}
	exist, err := dal.UpsertUser(id, target)
	if err != nil {
		return nil, writeError("INTERNAL_ERROR")
	}
	if exist.Valid() {
		if bytes.Equal(exist.PwdHash, pwd) {
			return exist, nil
		}
		if !register {
			return nil, writeError("WRONG_PASSWORD")
		}
		return nil, writeError("ALREADY_REGISTERED")
	}
	return target, nil
}

func doResetPassword(r *types.Request) error {
	if !r.User.IsRoot() {
		return writeError("MODS_REQUIRED")
	}
	_, err := dal.UpdateUser(r.Form.Get("id"), func(u *types.User) error {
		np := types.UUIDStr()
		h := hmac.New(sha1.New, []byte(types.Config.Key))
		h.Write([]byte(np))
		u.PwdHash = h.Sum(nil)
		u.LastResetPwd = np
		u.Session64 = rand.Int63()
		return nil
	})
	return err
}

func doUpdatePassword(r *types.Request) error {
	old := types.UTF16Trunc(r.Form.Get("old"), 100)
	new := types.UTF16Trunc(r.Form.Get("new"), 100)
	if new == "" {
		return writeError("INVALID_PASSWORD")
	}
	if old == new {
		return writeError("SAME_PASSWORD")
	}
	_, err := dal.UpdateUser(r.User.Id, func(u *types.User) error {
		h := hmac.New(sha1.New, []byte(types.Config.Key))
		h.Write([]byte(old))
		if !bytes.Equal(u.PwdHash, h.Sum(nil)) {
			return writeError("WRONG_PASSWORD")
		}
		h.Reset()
		h.Write([]byte(new))
		u.PwdHash = h.Sum(nil)
		u.Session64 = rand.Int63()
		return nil
	})
	return err
}
