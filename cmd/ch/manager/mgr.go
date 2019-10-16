package manager

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coyove/common/sched"
	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/manager/kv"
	mv "github.com/coyove/iis/cmd/ch/model"
)

var (
	rxCrawler = regexp.MustCompile(`(?i)(bot|googlebot|crawler|spider|robot|crawling)`)
)

type KeyValueOp interface {
	Lock(string)
	Unlock(string)
	Get(string) ([]byte, error)
	Set(string, []byte) error
	Delete(string) error
}

type Manager struct {
	db      KeyValueOp
	mu      sync.Mutex
	counter struct {
		m map[string]map[uint32]bool
		k sched.SchedKey
	}
}

func New(path string) (*Manager, error) {
	m := &Manager{
		//db: kv.NewBoltKV(path),
		db: kv.NewDynamoKV(config.Cfg.DyRegion, config.Cfg.DyAccessKey, config.Cfg.DySecretKey),
	}
	m.counter.m = map[string]map[uint32]bool{}
	return m, nil
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
		return
	}

	holes := 0

	for len(a) < n && root.Next != "" {
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
		go m.purgeDeleted(tag, root.ID)

		holes++
		if holes > 32 {
			log.Println("[mgr.Walk] Too many holes, started from [", cursor, "], now: [", next.ID, "], tag: [", tag, "]")
			break
		}

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
	for root := start; root.Next != ""; root = next {
		next, err = mv.UnmarshalTimeline(m.kvMustGet(root.Next))
		if err != nil {
			return
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
	log.Println("[mgr.purgeDeleted] New timeline:", start)
	m.db.Set(startID, start.Marshal())
}

func (m *Manager) Post(a *mv.Article) (string, error) {
	a.ID = ident.NewID().String()
	a.TimelineID = ident.NewID().String()

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return "", err
	}

	if err := m.insertTag(a, "", a.TimelineID); err != nil {
		return "", err
	}

	if err := m.insertTag(a, "#"+a.Category, ""); err != nil {
		return "", err
	}

	if err := m.insertTag(a, a.Author, ""); err != nil {
		return "", err
	}

	return a.ID, nil
}

func (m *Manager) insertTag(a *mv.Article, tag string, tlid string) error {
	rootID := ident.NewTagID(tag).String()

	m.db.Lock(rootID)
	defer m.db.Unlock(rootID)

	root, _ := mv.UnmarshalTimeline(m.kvMustGet(rootID))
	if root == nil {
		root = &mv.Timeline{
			ID: rootID,
		}
	}

	tl := &mv.Timeline{
		ID:   tlid,
		Next: root.Next,
		Ptr:  a.ID,
	}

	if tl.ID == "" {
		tl.ID = ident.NewID().String()
	}

	if err := m.db.Set(tl.ID, tl.Marshal()); err != nil {
		return err
	}

	root.Next = tl.ID
	if err := m.db.Set(root.ID, root.Marshal()); err != nil {
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
		// Move parent to the front
		p.TimelineID = ident.NewID().String()
		if err := m.insertTag(p, "", p.TimelineID); err != nil {
			return "", err
		}
	}

	// Save parent
	if err := m.db.Set(p.ID, p.Marshal()); err != nil {
		return "", err
	}

	if err := m.insertTag(a, a.Author, ""); err != nil {
		return "", err
	}

	if err := m.insertTag(a, p.Author, ""); err != nil {
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
	if a.Category != "" {
		m.insertTag(a, "#"+a.Category, "")
	}
	return m.db.Set(a.ID, a.Marshal())
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
