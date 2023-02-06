package main

import (
	"bytes"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"github.com/coyove/sdss/contrib/ngram"
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
			types.DedupUint64(append(ad.hash, ngram.StrHash(r.UserDisplay))))
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
			return writeError("PENDING_REVIEW")
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
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	}); err != nil {
		return err
	}

	time.AfterFunc(time.Second*10, func() { dal.UploadS3(ad.image, imageThumbName(ad.image)) })
	logrus.Infof("update %d, data: %v", id, ad)
	return nil
}

func doApprove(id uint64, r *types.Request) error {
	var target *types.Note
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

		dal.ProcessParentChanges(tx, target, target.ParentIds, target.ReviewParentIds)

		target.Title = target.ReviewTitle
		target.Content = target.ReviewContent
		target.Image = target.ReviewImage
		target.ParentIds = target.ReviewParentIds
		target.Reviewer = r.UserDisplay
		target.ClearReviewStatus()
		dal.UpdateCreator(tx, target.Creator, target)

		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	})
	if err == nil {
		h := buildBitmapHashes(target.Title, target.Creator, target.ParentIds)
		dal.Store.Saver().AddAsync(bitmap.Uint64Key(target.Id), types.DedupUint64(h))
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
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
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
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	})
}

func doDeleteHide(hide bool, r *types.Request) error {
	ids := types.DedupUint64(types.SplitUint64List(r.Form.Get("ids")))

	if err := dal.Store.Update(func(tx *bbolt.Tx) error {
		for _, id := range ids {
			n := dal.GetNoteTx(tx, id)
			if !n.Valid() {
				continue
			}
			if !r.User.IsMod() {
				if r.UserDisplay != n.Creator {
					return writeError("MODS_REQUIRED")
				}
				if clock.UnixMilli()-n.CreateUnix > 300*1000 {
					return writeError("DELETE_CHANCE_EXPIRED")
				}
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
		return dal.KSVUpsert(tx, dal.NoteBK, dal.KSVFromTag(target))
	})
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
