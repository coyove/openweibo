package dal

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal/kv"
	"github.com/coyove/iis/dal/weak_cache"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
)

var m struct {
	db        KeyValueOp
	weakUsers *weak_cache.Cache
}

func Init(region string, ak, sk string) {
	var db KeyValueOp

	if region == "" {
		db = kv.NewDiskKV()
	} else {
		db = kv.NewDynamoKV(region, ak, sk)
	}

	m.db = db
	m.weakUsers = weak_cache.NewCache(65536, time.Second)
}

func ModKV() KeyValueOp {
	return m.db
}

func GetArticle(id string) (*model.Article, error) {
	if id == "" {
		return nil, fmt.Errorf("empty ID")
	}
	p, err := m.db.Get(id)
	if err != nil {
		return nil, err
	}
	if len(p) == 0 {
		return nil, model.ErrNotExisted
	}
	a, err := model.UnmarshalArticle(p)
	if err != nil {
		return nil, err
	}
	if a.ReferID == "" {
		return a, nil
	}
	a2, err := GetArticle(a.ReferID)
	if err != nil {
		return nil, err
	}
	a2.NextID = a.NextID
	a2.NextMediaID = a.NextMediaID
	return a2, nil
}

func WalkMulti(media bool, n int, cursors ...ik.ID) (a []*model.Article, next []ik.ID) {
	if len(cursors) == 0 {
		return
	}

	startTime := time.Now()
	idm := map[string]bool{}

	for len(a) < n {
		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.WalkMulti] Break out slow walk at", cursors)
			break
		}

	DEDUP: // not some very good dedup code
		dedup := make(map[ik.ID]bool, len(cursors))
		for i, c := range cursors {
			if dedup[c] {
				cursors = append(cursors[:i], cursors[i+1:]...)
				goto DEDUP
			}
			dedup[c] = true
		}

		sort.Slice(cursors, func(i, j int) bool {
			if ii, jj := cursors[i].Time(), cursors[j].Time(); ii == jj {
				return bytes.Compare(cursors[i].TagBytes(), cursors[j].TagBytes()) < 0
			} else if cursors[i].IsRoot() { // i is bigger than j
				return false
			} else if cursors[j].IsRoot() {
				return true
			} else {
				return ii.Before(jj)
			}
		})

		latest := &cursors[len(cursors)-1]
		if !latest.Valid() {
			break
		}

		p, err := GetArticle(latest.String())
		if err == nil {
			if !idm[p.ID] && p.Content != model.DeletionMarker && !latest.IsRoot() {
				// 1. 'p' is not duplicated
				// 2. 'p' is not deleted
				// 3. 'p' is not a root article
				a = append(a, p)
				idm[p.ID] = true
			}
			*latest = ik.ParseID(p.PickNextID(media))
		} else {
			if err != model.ErrNotExisted {
				log.Println("[mgr.WalkMulti] Failed to get:", latest.String(), err)
			}

			*latest = ik.ID{}
		}
	}

	return a, cursors
}

func WalkReply(n int, cursor string) (a []*model.Article, next string) {
	startTime := time.Now()

	for len(a) < n && cursor != "" {
		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.WalkReply] Break out slow walk at", cursor)
			break
		}

		p, err := GetArticle(cursor)
		if err != nil {
			log.Println("[mgr.WalkReply] Failed to get:", cursor, err)
			break
		}

		if p.Content != model.DeletionMarker {
			a = append(a, p)
		}
		cursor = p.NextReplyID
	}

	return a, cursor
}

func WalkLikes(media bool, n int, cursor string) (a []*model.Article, next string) {
	startTime := time.Now()

	for len(a) < n && cursor != "" {
		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.WalkLikes] Break out slow walk at", cursor)
			break
		}

		p, err := GetArticle(cursor)
		if err != nil {
			log.Println("[mgr.WalkLikes] Failed to get:", cursor, err)
			break
		}

		if p.Extras["like"] == "true" {
			a2, err := GetArticle(p.Extras["to"])
			if err == nil {
				a2.NextID = p.NextID
				a = append(a, a2)
			} else {
				log.Println("[mgr.WalkLikes] Failed to get:", p.Extras["to"], err)
			}
		}

		cursor = p.PickNextID(media)
	}

	return a, cursor
}

func Post(a *model.Article, author *model.User, noMaster bool) (*model.Article, error) {
	a.ID = ik.NewGeneralID().String()
	a.CreateTime = time.Now()
	a.Author = author.ID

	if err := insertArticle(ik.NewID(ik.IDTagAuthor).SetTag(a.Author).String(), a, false); err != nil {
		return nil, err
	}

	if !noMaster {
		go insertArticle(ik.NewID(ik.IDTagAuthor).SetTag("master").String(), &model.Article{
			ID:         ik.NewGeneralID().String(),
			ReferID:    a.ID,
			Media:      a.Media,
			CreateTime: time.Now(),
		}, false)
	}

	go func() {
		ids, tags := common.ExtractMentionsAndTags(a.Content)
		MentionUserAndTags(a, ids, tags)
	}()

	return a, nil
}

func insertArticle(rootID string, a *model.Article, asReply bool) error {
	common.LockKey(rootID)
	defer common.UnlockKey(rootID)

	root, err := GetArticle(rootID)
	if err != nil && err != model.ErrNotExisted {
		return err
	}

	if err == model.ErrNotExisted {
		if asReply {
			return err
		}
		root = &model.Article{
			ID:         rootID,
			EOC:        a.ID,
			CreateTime: time.Now(),
		}
	}

	if x, y := ik.ParseID(rootID), ik.ParseID(root.NextID); x.Header() == ik.IDTagAuthor && y.Valid() {
		now := time.Now()
		if now.Year() != y.Time().Year() || now.Month() != y.Time().Month() {
			// The very last article was made before this month, so we will create a checkpoint for long jmp
			go func() {
				a := &model.Article{
					ID:         makeCheckpointID(x.Tag(), root.CreateTime),
					ReferID:    root.NextID,
					CreateTime: time.Now(),
				}
				m.db.Set(a.ID, a.Marshal())
			}()
		}
	}

	if asReply {
		a.NextReplyID, root.ReplyChain = root.ReplyChain, a.ID
	} else {
		a.NextID, root.NextID = root.NextID, a.ID
		if a.Media != "" {
			a.NextMediaID, root.NextMediaID = root.NextMediaID, a.ID
		}
	}
	root.Replies++

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return err
	}

	if err := m.db.Set(root.ID, root.Marshal()); err != nil {
		return err
	}
	return nil
}

func PostReply(parent string, content, media string, author *model.User, ip string, nsfw bool, noTimeline bool) (*model.Article, error) {
	p, err := GetArticle(parent)
	if err != nil {
		return nil, err
	}

	if p.Locked {
		return nil, fmt.Errorf("locked parent")
	}

	if IsBlocking(p.Author, author.ID) {
		return nil, fmt.Errorf("author blocked")
	}

	a := &model.Article{
		ID:         ik.NewGeneralID().String(),
		Content:    content,
		Media:      media,
		NSFW:       nsfw,
		Author:     author.ID,
		IP:         ip,
		Parent:     p.ID,
		CreateTime: time.Now(),
	}

	if err := insertArticle(p.ID, a, true); err != nil {
		return nil, err
	}

	if !noTimeline {
		// Add reply to its timeline
		if err := insertArticle(ik.NewID(ik.IDTagAuthor).SetTag(a.Author).String(), a, false); err != nil {
			return nil, err
		}
	}

	go func() {
		if p.Content != model.DeletionMarker && a.Author != p.Author {
			if err := insertArticle(ik.NewID(ik.IDTagInbox).SetTag(p.Author).String(), &model.Article{
				ID:  ik.NewGeneralID().String(),
				Cmd: model.CmdReply,
				Extras: map[string]string{
					"from":       a.Author,
					"article_id": a.ID,
				},
				CreateTime: time.Now(),
			}, false); err != nil {
				log.Println("PostReply", err)
			}

			UpdateUser(p.Author, func(u *model.User) error {
				u.Unread++
				return nil
			})
		}
		ids, tags := common.ExtractMentionsAndTags(a.Content)
		MentionUserAndTags(a, ids, tags)
	}()

	return a, nil
}

func UpdateArticle(aid string, cb func(a *model.Article) error) error {
	common.LockKey(aid)
	defer common.UnlockKey(aid)

	a, err := GetArticle(aid)
	if err != nil {
		return err
	}
	if err := cb(a); err != nil {
		return err
	}
	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return err
	}
	return nil
}
