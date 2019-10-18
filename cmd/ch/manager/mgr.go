package manager

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager/kv"
	mv "github.com/coyove/iis/cmd/ch/model"
)

var _Root = ident.NewTagID("").String()

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
	p, err := m.db.Get(id)
	if err != nil {
		return []byte("#ERR: " + err.Error())
	}
	if id == _Root {
		log.Printf("[TOOR] %s\n", string(p))
	}
	return p
}

func (m *Manager) Walk(tag string, cursor string, n int) (a []*mv.Article, nextID string, err error) {
	var root *mv.Timeline

	if cursor == "" {
		root, err = mv.UnmarshalTimeline(m.kvMustGet(ident.NewTagID(tag).String()))
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
			if strings.HasPrefix(tag, "#") && p.Category != tag[1:] {
				log.Println("[mgr.Walk] Stale tag ptr:", next, "expect:", tag, "actual:", p.Category)
				goto HOLE
			}

			if tag == "" && next.ID != p.TimelineID {
				log.Println("[mgr.Walk] Stale timeline ptr:", next, "actual:", p.TimelineID)
				goto HOLE
			}

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
			go m.purgeDeleted(tag, root.ID)
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

func (m *Manager) purgeDeleted(tag string, startID string) {
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
			if strings.HasPrefix(tag, "#") && p.Category != tag[1:] {
				// Refer to Walk()
			} else if tag == "" && next.ID != p.TimelineID {
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
	a.ID = ident.NewID().String()

	if err := m.insertRootThenUpdate(a); err != nil {
		return "", err
	}

	if err := m.insertTag(a.ID, "#"+a.Category); err != nil {
		// If failed, the article will be visible in timeline
		return "", err
	}

	if err := m.insertTag(a.ID, a.Author); err != nil {
		// If failed, the article will be visible in timeline and tagline
		return "", err
	}

	return a.ID, nil
}

func (m *Manager) insertTag(aid, tag string) error {
	rootID := ident.NewTagID(tag).String()

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
		ID:   ident.NewID().String(),
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

func (m *Manager) insertRootThenUpdate(a *mv.Article) error {
	rootID := ident.NewTagID("").String()

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

	a.TimelineID = ident.NewID().String()

	tl := &mv.Timeline{
		ID:   a.TimelineID,
		Ptr:  a.ID,
		Next: root.Next,
	}

	if err := m.db.Set(tl.ID, tl.Marshal()); err != nil {
		return err
	}

	root.Next = tl.ID
	if err := m.db.Set(root.ID, root.Marshal()); err != nil {
		// If failed, the newly created timeline marker 'tl' will be orphaned in the database forever
		return err
	}

	// At this stage, Walk() may find the marker not pointing to a.ID because we haven't updated a.TimelineID yet.
	// So the marker will be scheduled into purgeDeleted(), but since we are holding the lock of root,
	// it will not be deleted anyway. When we release the lock, everything should be fine.

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		// If failed, let purgeDeleted do the job
		return err
	}

	return nil
}

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
	if !pid.RIndexAppend(int16(nextIndex)) {
		return "", fmt.Errorf("too deep")
	}

	a.Category = ""
	a.Title = "RE: " + p.Title
	a.ID = pid.String()

	// Save reply
	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return "", err
	}

	p.ReplyTime = time.Now()
	p.Replies = nextIndex

	if p.TimelineID != "" && !p.Saged {
		// Move parent to the front of the timeline
		if err := m.insertRootThenUpdate(p); err != nil {
			return "", err
		}

	} else {
		// Save parent
		if err := m.db.Set(p.ID, p.Marshal()); err != nil {
			return "", err
		}
	}

	if err := m.insertTag(a.ID, a.Author); err != nil {
		return "", err
	}

	if err := m.insertTag(a.ID, p.Author); err != nil {
		return "", err
	}

	return a.ID, nil
}

func (m *Manager) Get(id string) (a *mv.Article, err error) {
	return mv.UnmarshalArticle(m.kvMustGet(id))
}

func (m *Manager) Update(a *mv.Article) error {
	m.db.Lock(a.ID)
	defer m.db.Unlock(a.ID)

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return err
	}

	if a.Category != "" {
		m.insertTag(a.ID, "#"+a.Category)
	}
	return nil
}

func (m *Manager) Delete(a *mv.Article) error {
	m.db.Lock(a.ID)
	defer m.db.Unlock(a.ID)
	return m.db.Delete(a.ID)
}

func (m *Manager) GetReplies(parent string, start, end int) (a []*mv.Article) {
	for i := start; i < end; i++ {
		if i <= 0 || i >= 128*128 {
			continue
		}

		pid := ident.ParseID(parent)
		if !pid.RIndexAppend(int16(i)) {
			break
		}

		p, _ := m.Get(pid.String())
		if p == nil {
			p = &mv.Article{ID: pid.String()}
		}
		a = append(a, p)
	}
	return
}
