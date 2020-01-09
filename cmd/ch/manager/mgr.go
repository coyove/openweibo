package manager

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager/kv"
	"github.com/coyove/iis/cmd/ch/mv"
	"github.com/coyove/iis/cmd/ch/weak_cache"
)

type Manager struct {
	db        KeyValueOp
	weakUsers *weak_cache.Cache
}

func New(path string) *Manager {
	var db KeyValueOp

	if config.Cfg.DyRegion == "" {
		db = kv.NewBoltKV(path)
	} else {
		db = kv.NewDynamoKV(config.Cfg.DyRegion, config.Cfg.DyAccessKey, config.Cfg.DySecretKey)
	}

	m := &Manager{db: db}
	m.weakUsers = weak_cache.NewCache(65536, time.Second)
	return m
}

func (m *Manager) ModKV() KeyValueOp {
	return m.db
}

func (m *Manager) GetArticle(id string) (*mv.Article, error) {
	if id == "" {
		return nil, fmt.Errorf("empty ID")
	}
	p, err := m.db.Get(id)
	if err != nil {
		return nil, err
	}
	if len(p) == 0 {
		return nil, mv.ErrNotExisted
	}
	a, err := mv.UnmarshalArticle(p)
	if err != nil {
		return nil, err
	}
	if a.ReferID == "" {
		return a, nil
	}
	a2, err := m.GetArticle(a.ReferID)
	if err != nil {
		return nil, err
	}
	a2.NextID = a.NextID
	a2.NextMediaID = a.NextMediaID
	return a2, nil
}

func (m *Manager) WalkMulti(media bool, n int, cursors ...ident.ID) (a []*mv.Article, next []ident.ID) {
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
		dedup := make(map[ident.ID]bool, len(cursors))
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

		p, err := m.GetArticle(latest.String())
		if err == nil {
			if !idm[p.ID] && p.Content != mv.DeletionMarker && !latest.IsRoot() {
				// 1. 'p' is not duplicated
				// 2. 'p' is not deleted
				// 3. 'p' is not a root article
				a = append(a, p)
				idm[p.ID] = true
			}
			*latest = ident.ParseID(p.PickNextID(media))
		} else {
			if err != mv.ErrNotExisted {
				log.Println("[mgr.WalkMulti] Failed to get:", latest.String(), err)
			}

			*latest = ident.ID{}
		}
	}

	return a, cursors
}

func (m *Manager) WalkReply(n int, cursor string) (a []*mv.Article, next string) {
	startTime := time.Now()

	for len(a) < n && cursor != "" {
		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.WalkReply] Break out slow walk at", cursor)
			break
		}

		p, err := m.GetArticle(cursor)
		if err != nil {
			log.Println("[mgr.WalkReply] Failed to get:", cursor, err)
			break
		}

		if p.Content != mv.DeletionMarker {
			a = append(a, p)
		}
		cursor = p.NextReplyID
	}

	return a, cursor
}

func (m *Manager) WalkLikes(media bool, n int, cursor string) (a []*mv.Article, next string) {
	startTime := time.Now()

	for len(a) < n && cursor != "" {
		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.WalkLikes] Break out slow walk at", cursor)
			break
		}

		p, err := m.GetArticle(cursor)
		if err != nil {
			log.Println("[mgr.WalkLikes] Failed to get:", cursor, err)
			break
		}

		if p.Extras["like"] == "true" {
			a2, err := m.GetArticle(p.Extras["to"])
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

func (m *Manager) Post(a *mv.Article, author *mv.User) (string, error) {
	a.ID = ident.NewGeneralID().String()
	a.CreateTime = time.Now()
	a.Author = author.ID

	if err := m.insertArticle(ident.NewID(ident.IDTagAuthor).SetTag(a.Author).String(), a, false); err != nil {
		return "", err
	}

	if !author.Settings().NoPostInMaster {
		go m.insertArticle(ident.NewID(ident.IDTagAuthor).SetTag("master").String(), &mv.Article{
			ID:         ident.NewGeneralID().String(),
			ReferID:    a.ID,
			Media:      a.Media,
			CreateTime: time.Now(),
		}, false)
	}

	go func() {
		ids, tags := mv.ExtractMentionsAndTags(a.Content)
		m.MentionUserAndTags(a, ids, tags)
	}()

	return a.ID, nil
}

func (m *Manager) insertArticle(rootID string, a *mv.Article, asReply bool) error {
	m.db.Lock(rootID)
	defer m.db.Unlock(rootID)

	root, err := m.GetArticle(rootID)
	if err != nil && err != mv.ErrNotExisted {
		return err
	}

	if err == mv.ErrNotExisted {
		if asReply {
			return err
		}
		root = &mv.Article{
			ID:         rootID,
			EOC:        a.ID,
			CreateTime: time.Now(),
		}
	}

	if x, y := ident.ParseID(rootID), ident.ParseID(root.NextID); x.Header() == ident.IDTagAuthor && y.Valid() {
		now := time.Now()
		if now.Year() != y.Time().Year() || now.Month() != y.Time().Month() {
			// The very last article was made before this month, so we will create a checkpoint for long jmp
			go func() {
				a := &mv.Article{
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

func (m *Manager) PostReply(parent string, content, media string, author *mv.User, ip string, nsfw bool) (string, error) {
	p, err := m.GetArticle(parent)
	if err != nil {
		return "", err
	}

	if p.Locked {
		return "", fmt.Errorf("locked parent")
	}

	if m.IsBlocking(p.Author, author.ID) {
		return "", fmt.Errorf("author blocked")
	}

	a := &mv.Article{
		ID:         ident.NewGeneralID().String(),
		Content:    content,
		Media:      media,
		NSFW:       nsfw,
		Author:     author.ID,
		IP:         ip,
		Parent:     p.ID,
		CreateTime: time.Now(),
	}

	if err := m.insertArticle(p.ID, a, true); err != nil {
		return "", err
	}

	if !author.Settings().NoReplyInTimeline {
		// Add reply to its timeline
		if err := m.insertArticle(ident.NewID(ident.IDTagAuthor).SetTag(a.Author).String(), a, false); err != nil {
			return "", err
		}
	}

	go func() {
		if p.Content != mv.DeletionMarker && a.Author != p.Author {
			if err := m.insertArticle(ident.NewID(ident.IDTagInbox).SetTag(p.Author).String(), &mv.Article{
				ID:  ident.NewGeneralID().String(),
				Cmd: mv.CmdReply,
				Extras: map[string]string{
					"from":       a.Author,
					"article_id": a.ID,
				},
				CreateTime: time.Now(),
			}, false); err != nil {
				log.Println("PostReply", err)
			}

			m.UpdateUser(p.Author, func(u *mv.User) error {
				u.Unread++
				return nil
			})
		}
		ids, tags := mv.ExtractMentionsAndTags(a.Content)
		m.MentionUserAndTags(a, ids, tags)
	}()

	return a.ID, nil
}

func (m *Manager) UpdateArticle(aid string, cb func(a *mv.Article) error) error {
	m.db.Lock(aid)
	defer m.db.Unlock(aid)

	a, err := m.GetArticle(aid)
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
