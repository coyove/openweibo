package manager

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager/kv"
	mv "github.com/coyove/iis/cmd/ch/model"
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

func (m *Manager) NewPost(title, content, author, ip string, cat string) *mv.Article {
	return &mv.Article{
		Title:      title,
		Content:    content,
		Author:     author,
		Category:   cat,
		IP:         ip,
		CreateTime: time.Now(),
		ReplyTime:  time.Now(),
	}
}

func (m *Manager) NewReply(content, author, ip string) *mv.Article {
	return m.NewPost("", content, author, ip, "")
}

func (m *Manager) kvMustGet(id string) []byte {
	if id == "" {
		return []byte("{}")
	}

	p, err := m.db.Get(id)
	if err != nil {
		return []byte("#ERR: " + err.Error())
	}

	return p
}

func (m *Manager) Walk(hdr ident.IDTag, tag string, cursor string, n int) (a []*mv.Article, nextID string, err error) {
	var root *mv.Timeline

	if cursor == "" {
		root, err = mv.UnmarshalTimeline(m.kvMustGet(ident.NewID(hdr).SetTag(tag).String()))
	} else {
		root, err = mv.UnmarshalTimeline(m.kvMustGet(cursor))
	}
	if err != nil {
		if !strings.Contains(err.Error(), "#ERR") {
			err = nil
		}
		return
	}

	holes := map[string]bool{}
	startTime := time.Now()

	for len(a) < n && root.Next != "" {

		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.Walk] Break out slow walk from: [", cursor, "], now at: ", root.ID, ", tag: [", tag, "]")
			break
		}

		next, err := mv.UnmarshalTimeline(m.kvMustGet(root.Next))
		if err != nil {
			return nil, "", err
		}

		p, err := mv.UnmarshalArticle(m.kvMustGet(next.Ptr))
		if err == nil {
			//if hdr == ident.IDTagCategory && p.Category != tag {
			//	log.Println("[mgr.Walk] Stale tag ptr:", next, "expect:", tag, "actual:", p.Category)
			//	goto HOLE
			//}

			//if tag == "" && next.ID != p.TimelineID {
			//	log.Println("[mgr.Walk] Stale timeline ptr:", next, "actual:", p.TimelineID)
			//	goto HOLE
			//}

			//if tag == "" && strings.HasPrefix(p.Category, "!") {
			//	log.Println("[mgr.Walk] Stale !category ptr:", next, "actual:", p.Category)
			//	goto HOLE
			//}

			a = append(a, p)
		} else {
			log.Println("[mgr.Walk] Stale pointer:", next, err)
			goto HOLE
		}

		root = next
		continue

	HOLE:
		// root.Next should be unlinked and root's next will be root.Next.Next, but since root itself will also be deleted
		// there is no need to schedule it into purgeDeleted()
		if !holes[root.ID] {
			go m.purgeDeleted(hdr, tag, root.ID)
		}

		holes[root.Next] = true
		root = next
	}

	nextID = root.ID
	if nextID == cursor {
		nextID = ""
	}

	return
}

func (m *Manager) WalkMulti(n int, cursors ...ident.ID) (a []*mv.Article, next []ident.ID) {
	startTime := time.Now()

	/* Check cursors */
	{
		i := 0
		for _, c := range cursors {
			if c.IsZeroTime() {
				i++
			}
		}

		// Cursors must be all root starters or not
		if i != 0 && i != len(cursors) {
			log.Println("[mgr.WalkMulti] Invalid cursors:", cursors)
			return
		}

		if i == len(cursors) {
			// If all cursors are root starters, translate them into the latest timeline markers (root.Next)
			for i, root := range cursors {
				if time.Since(startTime).Seconds() > 0.5 {
					log.Println("[mgr.WalkMulti] Break out slow ROOT walk at", cursors)
					break
				}

				tl, err := mv.UnmarshalTimeline(m.kvMustGet(root.String()))
				if err != nil {
					log.Println("[mgr.WalkMulti] Get root:", root, err)
					continue
				}
				cursors[i] = ident.ParseID(tl.Next)
			}
		}

		i = 0
		for i, c := range cursors {
			if c.IsZeroTime() || !c.Valid() {
				cursors[i] = ident.ID{}
				i++
			}
		}

		if len(cursors) == i {
			return
		}
	}

	for len(a) < n {
		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.WalkMulti] Break out slow walk at [", cursors, "]")
			break
		}

		sort.Slice(cursors, func(i, j int) bool {
			if cursors[i].Time() == cursors[j].Time() {
				return cursors[i].Tag() < cursors[j].Tag()
			}
			return cursors[i].Time().Before(cursors[j].Time())
		})

		latest := &cursors[len(cursors)-1]
		if !latest.Valid() {
			break
		}

		tl, err := mv.UnmarshalTimeline(m.kvMustGet(latest.String()))
		if err != nil {
			log.Println("[mgr.WalkMulti] Failed to get timeline marker:", *latest, err)
			*latest = ident.ID{}
			continue
		}

		p, err := mv.UnmarshalArticle(m.kvMustGet(tl.Ptr))
		if err == nil {
			a = append(a, p)
		} else {
			log.Println("[mgr.WalkMulti] Failed to get:", tl, err)
			// go m.purgeDeleted(hdr, tag, root.ID)
		}
		*latest = ident.ParseID(tl.Next)
	}

	return a, cursors
}

func (m *Manager) purgeDeleted(hdr ident.IDTag, tag string, startID string) {
	m.db.Lock(startID)
	defer m.db.Unlock(startID)

	start, err := mv.UnmarshalTimeline(m.kvMustGet(startID))
	if err != nil {
		return
	}

	var next *mv.Timeline
	startTime := time.Now()
	oldNext := start.Next

	for root := start; root.Next != ""; root = next {
		next, err = mv.UnmarshalTimeline(m.kvMustGet(root.Next))
		if err != nil {
			return
		}

		if time.Since(startTime).Seconds() > 10 {
			start.Next = next.ID
			goto SET
		}

		p, err := mv.UnmarshalArticle(m.kvMustGet(next.Ptr))
		if err == nil {
			if tag == "" && next.ID != p.TimelineID {
				// Refer to Walk()
			} else {
				start.Next = next.ID
				goto SET
			}
		}
	}

	start.Next = ""

SET:
	if start.Next == oldNext {
		return
	}

	log.Println("[mgr.purgeDeleted] New next of", start, ", old:", oldNext)
	m.db.Set(startID, start.Marshal())
}

func (m *Manager) Post(a *mv.Article) (string, error) {
	a.ID = ident.NewGeneralID().String()

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		// If failed, the article will be visible in timeline and tagline
		return "", err
	}

	if err := m.insert(a.ID, ident.IDTagAuthor, a.Author); err != nil {
		// If failed, the article will be visible in timeline and tagline
		return "", err
	}

	go m.UpdateUser(a.Author, func(u *mv.User) error {
		u.TotalPosts++
		return nil
	})

	for _, id := range mv.ExtractMentions(a.Content) {
		go m.MentionUser(a, id)
	}

	return a.ID, nil
}

func (m *Manager) insert(aid string, hdr ident.IDTag, tag string) error {
	rootID := ident.NewID(hdr).SetTag(tag).String()

	m.db.Lock(rootID)
	defer m.db.Unlock(rootID)

	root, err := mv.UnmarshalTimeline(m.kvMustGet(rootID))
	if err != nil && strings.Contains(err.Error(), "#ERR") {
		return err
	}

	if root == nil {
		root = &mv.Timeline{
			ID: rootID,
		}
	}

	tl := &mv.Timeline{
		ID:   ident.NewID(hdr).SetTag(tag).SetTime(time.Now()).String(),
		Next: root.Next,
		Ptr:  aid,
	}

	if err := m.db.Set(tl.ID, tl.Marshal()); err != nil {
		return err
	}

	root.Next = tl.ID
	if err := m.db.Set(root.ID, root.Marshal()); err != nil {
		// If failed, the stale timeline marker 'tl' will be left in the database
		return err
	}

	return nil
}

// func (m *Manager) insertRootThenUpdate(a *mv.Article) error {
// 	rootID := ident.NewTagID(ident.IDTagGeneral, nil).String()
//
// 	m.db.Lock(rootID)
// 	defer m.db.Unlock(rootID)
//
// 	root, err := mv.UnmarshalTimeline(m.kvMustGet(rootID))
// 	if err != nil && strings.Contains(err.Error(), "#ERR") {
// 		return err
// 	}
//
// 	if root == nil {
// 		root = &mv.Timeline{
// 			ID: rootID,
// 		}
// 	}
//
// 	a.TimelineID = ident.NewID().String()
//
// 	tl := &mv.Timeline{
// 		ID:   a.TimelineID,
// 		Ptr:  a.ID,
// 		Next: root.Next,
// 	}
//
// 	if strings.HasPrefix(a.Category, "!") {
// 		goto PUT
// 	}
//
// 	if err := m.db.Set(tl.ID, tl.Marshal()); err != nil {
// 		return err
// 	}
//
// 	root.Next = tl.ID
// 	if err := m.db.Set(root.ID, root.Marshal()); err != nil {
// 		// If failed, the newly created timeline marker 'tl' will be orphaned in the database forever
// 		return err
// 	}
//
// 	// At this stage, Walk() may find the marker not pointing to a.ID because we haven't updated a.TimelineID yet.
// 	// So the marker will be scheduled into purgeDeleted(), but since we are holding the lock of root,
// 	// it will not be deleted anyway. When we release the lock, everything should be fine.
//
// PUT:
// 	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
// 		// If failed, let purgeDeleted do the job
// 		return err
// 	}
//
// 	return nil
// }

func (m *Manager) PostReply(parent string, a *mv.Article) (string, error) {
	m.db.Lock(parent)
	defer m.db.Unlock(parent)

	p, err := m.Get(parent)
	if err != nil {
		return "", err
	}
	if p.Locked {
		return "", fmt.Errorf("locked parent")
	}
	if p.Replies >= 16000 {
		return "", fmt.Errorf("too many replies")
	}

	pid := ident.ParseID(parent)

	nextIndex := p.Replies + 1
	pid = pid.SetReply(uint16(nextIndex))

	a.Category = ""
	a.Title = "RE: " + p.Title
	a.ID = pid.String()

	// Save reply
	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return "", err
	}

	p.ReplyTime = time.Now()
	p.Replies = nextIndex

	// Save parent
	if err := m.db.Set(p.ID, p.Marshal()); err != nil {
		return "", err
	}

	if err := m.insert(a.ID, ident.IDTagInbox, p.Author); err != nil {
		return "", err
	}

	go m.UpdateUser(a.Author, func(u *mv.User) error {
		u.TotalPosts++
		return nil
	})

	go m.UpdateUser(p.Author, func(u *mv.User) error {
		u.Unread++
		return nil
	})

	for _, id := range mv.ExtractMentions(a.Content) {
		go m.MentionUser(a, id)
	}

	return a.ID, nil
}

func (m *Manager) Get(id string) (a *mv.Article, err error) {
	return mv.UnmarshalArticle(m.kvMustGet(id))
}

func (m *Manager) Update(a *mv.Article, oldcat ...string) error {
	m.db.Lock(a.ID)
	defer m.db.Unlock(a.ID)

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return err
	}

	return nil
}

func (m *Manager) Delete(a *mv.Article) error {
	m.db.Lock(a.ID)
	defer m.db.Unlock(a.ID)
	return m.db.Delete(a.ID)
}

func (m *Manager) GetReplies(parent string, start, end int) (a []*mv.Article) {
	pid := ident.ParseID(parent)

	for i := start; i < end; i++ {
		if i <= 0 || i >= 128*128 {
			continue
		}

		pid = pid.SetReply(uint16(i))
		p, _ := m.Get(pid.String())
		if p == nil {
			p = &mv.Article{ID: pid.String()}
		}
		a = append(a, p)
	}
	return
}
