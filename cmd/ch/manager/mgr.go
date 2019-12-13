package manager

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager/kv"
	"github.com/coyove/iis/cmd/ch/mv"
)

type Manager struct {
	db KeyValueOp
}

func New(path string) *Manager {
	var db KeyValueOp

	if config.Cfg.DyRegion == "" {
		db = kv.NewBoltKV(path)
	} else {
		db = kv.NewDynamoKV(config.Cfg.DyRegion, config.Cfg.DyAccessKey, config.Cfg.DySecretKey)
	}

	m := &Manager{db: db}
	return m
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
	return a2, nil
}

func (m *Manager) WalkMulti(n int, cursors ...ident.ID) (a []*mv.Article, next []ident.ID) {
	if len(cursors) == 0 {
		return
	}

	startTime := time.Now()
	for i, root := range cursors {
		if time.Since(startTime).Seconds() > 0.5 {
			log.Println("[mgr.WalkMulti] Break out slow cursor walk", cursors)
			break
		}

		if !root.IsRoot() {
			continue
		}

		tl, err := m.GetArticle(root.String())
		if err != nil {
			if err != mv.ErrNotExisted {
				log.Println("[mgr.Walk] Get root:", root, err)
			}
			cursors[i] = ident.ID{}
		} else {
			cursors[i] = ident.ParseID(tl.NextID)
		}
	}

	for len(a) < n {
		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.WalkMulti] Break out slow walk at", cursors)
			break
		}

	DEDUP:
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
				return cursors[i].Tag() < cursors[j].Tag()
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
			a = append(a, p)
			*latest = ident.ParseID(p.NextID)
		} else {
			log.Println("[mgr.WalkMulti] Failed to get:", latest.String(), err)
			// go m.purgeDeleted(hdr, tag, root.ID)
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
		if err == nil {
			a = append(a, p)
		} else {
			log.Println("[mgr.WalkReply] Failed to get:", cursor, err)
			// go m.purgeDeleted(hdr, tag, root.ID)
		}
		cursor = p.NextReplyID
	}

	return a, cursor
}

// func (m *Manager) purgeDeleted(hdr ident.IDTag, tag string, startID string) {
// 	m.db.Lock(startID)
// 	defer m.db.Unlock(startID)
//
// 	start, err := mv.UnmarshalTimeline(m.kvMustGet(startID))
// 	if err != nil {
// 		return
// 	}
//
// 	var next *mv.Timeline
// 	startTime := time.Now()
// 	oldNext := start.Next
//
// 	for root := start; root.Next != ""; root = next {
// 		next, err = mv.UnmarshalTimeline(m.kvMustGet(root.Next))
// 		if err != nil {
// 			return
// 		}
//
// 		if time.Since(startTime).Seconds() > 10 {
// 			start.Next = next.ID
// 			goto SET
// 		}
//
// 		p, err := mv.UnmarshalArticle(m.kvMustGet(next.Ptr))
// 		if err == nil {
// 			if tag == "" && next.ID != p.TimelineID {
// 				// Refer to Walk()
// 			} else {
// 				start.Next = next.ID
// 				goto SET
// 			}
// 		}
// 	}
//
// 	start.Next = ""
//
// SET:
// 	if start.Next == oldNext {
// 		return
// 	}
//
// 	log.Println("[mgr.purgeDeleted] New next of", start, ", old:", oldNext)
// 	m.db.Set(startID, start.Marshal())
// }

func (m *Manager) Ref(a *mv.Article, id string) error {
	return m.insertArticle(ident.NewID(ident.IDTagAuthor).SetTag(id).String(), a, false)
}

func (m *Manager) Post(content string, author *mv.User, ip string) (string, error) {
	a := &mv.Article{
		ID:         ident.NewGeneralID().String(),
		Content:    content,
		Author:     author.ID,
		IP:         ip,
		CreateTime: time.Now(),
	}

	if err := m.insertArticle(ident.NewID(ident.IDTagAuthor).SetTag(a.Author).String(), a, false); err != nil {
		// If failed, the article will be visible in timeline and tagline
		return "", err
	}

	if !author.NoPostInMaster {
		go m.Ref(&mv.Article{
			ID:         ident.NewGeneralID().String(),
			ReferID:    a.ID,
			CreateTime: time.Now(),
		}, "master")
	}

	go func() {
		m.UpdateUser(a.Author, func(u *mv.User) error {
			u.TotalPosts++
			return nil
		})
		for _, id := range mv.ExtractMentions(a.Content) {
			m.MentionUser(a, id)
		}
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
		root = &mv.Article{ID: rootID}
	}

	if asReply {
		a.NextReplyID, root.ReplyChain = root.ReplyChain, a.ID
		root.Replies++
	} else {
		a.NextID, root.NextID = root.NextID, a.ID
	}

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return err
	}

	if err := m.db.Set(root.ID, root.Marshal()); err != nil {
		return err
	}
	return nil
}

func (m *Manager) PostReply(parent string, content string, author *mv.User, ip string) (string, error) {
	p, err := m.Get(parent)
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
		Author:     author.ID,
		IP:         ip,
		Parent:     p.ID,
		CreateTime: time.Now(),
	}

	if err := m.insertArticle(p.ID, a, true); err != nil {
		return "", err
	}

	if !author.NoReplyInTimeline {
		// Add reply to its timeline
		if err := m.insertArticle(ident.NewID(ident.IDTagAuthor).SetTag(a.Author).String(), a, false); err != nil {
			return "", err
		}
	}

	if err := m.insertArticle(ident.NewID(ident.IDTagInbox).SetTag(p.Author).String(), &mv.Article{
		ID:  ident.NewGeneralID().String(),
		Cmd: mv.CmdReply,
		Extras: map[string]string{
			"from":       a.Author,
			"article_id": a.ID,
		},
		CreateTime: time.Now(),
	}, false); err != nil {
		return "", err
	}

	go func() {
		m.UpdateUser(p.Author, func(u *mv.User) error {
			u.Unread++
			return nil
		})
		for _, id := range mv.ExtractMentions(a.Content) {
			m.MentionUser(a, id)
		}
	}()

	return a.ID, nil
}

func (m *Manager) Get(id string) (a *mv.Article, err error) {
	return m.GetArticle(id)
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
