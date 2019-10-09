package manager

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/coyove/common/lru"
	"github.com/coyove/common/sched"
	"github.com/coyove/iis/cmd/ch/ident"
	mv "github.com/coyove/iis/cmd/ch/model"
	"github.com/etcd-io/bbolt"
)

var (
	errNoBucket = errors.New("")
	bkPost      = []byte("post")
)

type Manager struct {
	db      *bbolt.DB
	cache   *lru.Cache
	mu      sync.Mutex
	counter struct {
		m  map[string]map[uint32]bool
		k  sched.SchedKey
		rx *regexp.Regexp
	}
}

func New(path string) (*Manager, error) {
	db, err := bbolt.Open(path, 0700, &bbolt.Options{
		FreelistType: bbolt.FreelistMapType,
	})
	if err != nil {
		return nil, err
	}

	m := &Manager{
		db:    db,
		cache: lru.NewCache(128),
	}
	m.counter.m = map[string]map[uint32]bool{}
	m.counter.rx = regexp.MustCompile(`(?i)(bot|googlebot|crawler|spider|robot|crawling)`)

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bkPost)
		return err
	})
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manager) NewPost(title, content, author, ip string, cat string) *mv.Article {
	return &mv.Article{
		Title:      title,
		Content:    content,
		Author:     author,
		Category:   cat,
		IP:         ip,
		CreateTime: uint32(time.Now().Unix()),
		ReplyTime:  uint32(time.Now().Unix()),
	}
}

func (m *Manager) NewReply(content, author, ip string) *mv.Article {
	return &mv.Article{
		Content:    content,
		Author:     author,
		IP:         ip,
		CreateTime: uint32(time.Now().Unix()),
		ReplyTime:  uint32(time.Now().Unix()),
	}
}

func (m *Manager) Walk(tag string, cursor []byte, n int) (a []*mv.Article, prev []byte, next []byte, err error) {
	err = m.db.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(bkPost)

		var res [][2][]byte
		res, prev, next = scanBucketDesc(bk, tag, cursor, n)

		a = m.mget(bk, tag == "", res)

		if ident.ParseID(next).Tag() != tag {
			next = nil
		}

		return nil
	})
	return
}

func (m *Manager) insertTag(bk *bbolt.Bucket, b string, k, v []byte) error {
	kid := ident.ParseID(k)
	kid.SetTag(b)
	kid.SetHeader(ident.HeaderAuthorTag)
	return bk.Put(kid.Marshal(), v)
}

func (m *Manager) deleteTags(bk *bbolt.Bucket, k []byte, tags ...string) (err error) {
	kid := ident.ParseID(k)
	kid.SetHeader(ident.HeaderAuthorTag)
	for _, tag := range tags {
		kid.SetTag(tag)
		err = bk.Delete(kid.Marshal())
	}
	return
}

func (m *Manager) PostPost(a *mv.Article) ([]byte, error) {
	err := m.db.Update(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(bkPost)

		a.ID = ident.NewID(ident.HeaderArticle, "\x80").Marshal()
		if a.Announce {
			a.Timeline = ident.NewID(ident.HeaderAnnounce, "").Marshal()
		} else {
			a.Timeline = ident.NewID(ident.HeaderTimeline, "").Marshal()
		}

		if err := bk.Put(a.ID, a.MarshalA()); err != nil {
			return err
		}
		if err := bk.Put(a.Timeline, a.ID); err != nil {
			return err
		}
		if err := m.insertTag(bk, a.Author, a.ID, a.ID); err != nil {
			return err
		}
		if err := m.insertTag(bk, "#"+a.Category, a.ID, a.ID); err != nil {
			return err
		}

		return nil
	})
	return a.ID, err
}

func (m *Manager) PostReply(parent []byte, a *mv.Article) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, err := m.Get(parent)
	if err != nil {
		return nil, err
	}
	if p.Locked {
		return nil, fmt.Errorf("locked parent")
	}
	if p.Replies >= 16000 {
		return nil, fmt.Errorf("too many replies")
	}

	pid := ident.ParseID(parent)

	p.ReplyTime = uint32(time.Now().Unix())
	p.Replies++

	a.Category = ""
	a.Title = "RE: " + p.Title
	a.Index = p.Replies

	if !pid.RIndexAppend(int16(p.Replies)) {
		return nil, fmt.Errorf("too deep")
	}

	a.ID = pid.Marshal()

	m.cache.Remove(string(parent))
	return a.ID, m.db.Update(func(tx *bbolt.Tx) error {
		main := tx.Bucket(bkPost)
		if err := main.Put(a.ID, a.MarshalA()); err != nil {
			return err
		}

		if p.Timeline != nil && !p.Announce && !p.Saged {
			// Move the parent post to the front of the timeline
			if err := main.Delete(p.Timeline); err != nil {
				return err
			}
			p.Timeline = ident.NewID(ident.HeaderTimeline, "").Marshal()
			if err := main.Put(p.Timeline, p.ID); err != nil {
				return err
			}
		}
		if err := main.Put(p.ID, p.MarshalA()); err != nil {
			return err
		}
		if err := m.insertTag(main, a.Author, a.ID, a.ID); err != nil {
			return err
		}
		if p.Author != a.Author {
			return m.insertTag(main, p.Author, ident.NewID(ident.HeaderAuthorTag, "").Marshal(), a.ID)
		}
		return nil
	})
}

func (m *Manager) Get(id []byte) (a *mv.Article, err error) {
	m.db.View(func(tx *bbolt.Tx) error {
		a = m.get(tx.Bucket(bkPost), id)
		return nil
	})
	if a == nil || a.ID == nil {
		err = fmt.Errorf("not found")
	}
	return
}

func (m *Manager) Update(a *mv.Article, oldcat string) error {
	m.cache.Remove(string(a.ID))
	return m.db.Update(func(tx *bbolt.Tx) error {
		main := tx.Bucket(bkPost)

		if a.Category != oldcat {
			if err := m.deleteTags(main, a.ID, "#"+oldcat); err != nil {
				return err
			}
			if err := m.insertTag(main, "#"+a.Category, a.ID, a.ID); err != nil {
				return err
			}
		}

		return main.Put(a.ID, a.MarshalA())
	})
}

func (m *Manager) Delete(a *mv.Article) error {
	m.cache.Remove(string(a.ID))
	return m.db.Update(func(tx *bbolt.Tx) error {
		main := tx.Bucket(bkPost)
		if err := main.Delete(a.ID); err != nil {
			return err
		}
		return m.deleteTags(main, a.ID, "#"+a.Category, a.Author)
	})
}
