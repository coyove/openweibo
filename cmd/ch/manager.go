package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/etcd-io/bbolt"
)

var errNoBucket = errors.New("")

type Manager struct {
	db      *bbolt.DB
	mu      sync.Mutex
	counter int64
	home    string
	closed  bool
}

type Tag struct {
	Name  string
	Count int
}

func NewManager(path string) (*Manager, error) {
	db, err := bbolt.Open(path, 0700, &bbolt.Options{
		FreelistType: bbolt.FreelistMapType,
	})
	if err != nil {
		return nil, err
	}

	m := &Manager{
		db:      db,
		counter: rand.Int63(),
	}

	if err := db.Update(func(tx *bbolt.Tx) error {
		for _, bk := range bkNames {
			if _, err = tx.CreateBucketIfNotExists(bk); err != nil {
				return err
			}
		}
		m.home = string(tx.Bucket(bkPost).Get([]byte("home")))
		return nil
	}); err != nil {
		return nil, err
	}

	return m, nil
}

type findby struct {
	bkName  []byte
	bkName2 []byte
}

func ByTag(tag string) findby {
	return findby{
		bkName:  bkTag,
		bkName2: []byte(tag),
	}
}

func ByAuthor(a string) findby {
	return findby{
		bkName:  bkAuthor,
		bkName2: []byte(a),
	}
}

func ByTimeline() findby {
	return findby{
		bkName: bkPost,
	}
}

func (m *Manager) FindPosts(dir byte, filter findby, cursor int64, n int) ([]*Article, bool, int, error) {
	var (
		more  bool
		a     []*Article
		count int
		err   = m.db.View(func(tx *bbolt.Tx) error {
			bk := tx.Bucket(filter.bkName)
			if filter.bkName2 != nil {
				bk = bk.Bucket(filter.bkName2)
			}
			if bk == nil {
				return nil
			}

			count = bk.Stats().KeyN

			var res [][2][]byte
			var next []byte

			if dir == 'a' {
				if cursor == -1 {
					cursor = 0
				}
				res, next = ScanBucketAsc(bk, idBytes(cursor), n, true)
			} else {
				res, next = ScanBucketDesc(bk, idBytes(cursor), n, false)
			}

			more = next != nil
			a = mget(tx, bytes.Equal(filter.bkName, bkPost), res)

			if bytes.Equal(filter.bkName, bkPost) {
				for i := len(a) - 1; i >= 0; i-- {
					if a[i].Parent != 0 {
						a = append(a[:i], a[i+1:]...)
					}
				}
			}
			return nil
		})
	)
	return a, more, count, err
}

func (m *Manager) FindTags(cursor string, n int) ([]Tag, int) {
	var a []Tag
	var holes []string
	m.db.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(bkTag)
		res, _ := ScanBucketAsc(bk, []byte(cursor), n, false)

		for _, r := range res {
			bkt := bk.Bucket(r[0])
			t := Tag{Name: string(r[0])}
			if bkt != nil {
				t.Count = bkt.Stats().KeyN
				if t.Count == 0 {
					holes = append(holes, t.Name)
					continue
				}
			}
			a = append(a, t)
		}

		n = bk.Stats().BucketN
		return nil
	})

	if len(holes) > 0 {
		go func() {
			m.db.Update(func(tx *bbolt.Tx) error {
				bk := tx.Bucket(bkTag)
				for _, h := range holes {
					bk.DeleteBucket([]byte(h))
				}
				return nil
			})
		}()
	}

	return a, n
}

func (m *Manager) updateTags(tx *bbolt.Tx, id int64, tags ...string) error {
	bk := tx.Bucket(bkTag)
	for _, tag := range tags {
		bk, err := bk.CreateBucketIfNotExists([]byte(tag))
		if err != nil {
			return err
		}
		if err := bk.Put(idBytes(id), idBytes(id)); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) deleteTags(tx *bbolt.Tx, id int64, tags ...string) error {
	bk := tx.Bucket(bkTag)
	for _, tag := range tags {
		bk2 := bk.Bucket([]byte(tag))
		if bk2 == nil {
			continue
		}
		if err := bk2.Delete(idBytes(id)); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) appendUserInboxChain(tx *bbolt.Tx, user string, i, id int64) error {
	bk, err := tx.CreateBucketIfNotExists(bkAuthor)
	if err != nil {
		return err
	}
	bk, err = bk.CreateBucketIfNotExists([]byte(user))
	if err != nil {
		return err
	}
	err = bk.Put(idBytes(i), idBytes(id))
	if err != nil {
		return err
	}
	if bk.Stats().KeyN > config.InboxSize {
		k, _ := bk.Cursor().First()
		return bk.Delete(k)
	}
	return nil
}

func (m *Manager) PostPost(a *Article) error {
	return m.db.Update(func(tx *bbolt.Tx) error {
		idbuf := idBytes(a.ID)
		bk := tx.Bucket(bkPost)

		a.Index = int64(bk.Stats().KeyN + 1)

		if err := bk.Put(idbuf, a.marshal()); err != nil {
			return err
		}
		if err := m.appendUserInboxChain(tx, a.Author, a.ID, a.ID); err != nil {
			return err
		}
		if err := m.updateTags(tx, a.ID, a.Tags...); err != nil {
			return err
		}
		return nil
	})
}

func (m *Manager) PostReply(parent int64, a *Article) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, err := m.GetArticle(parent)
	if err != nil {
		return err
	}
	if p.Locked {
		return fmt.Errorf("locked parent")
	}
	if len(p.Replies) >= 8192 {
		return fmt.Errorf("too many replies")
	}
	if strings.Count(p.Title, "RE:") > 4 {
		return fmt.Errorf("too deep")
	}

	p.ReplyTime = uint32(time.Now().Unix())
	p.Replies = append(p.Replies, a.ID)
	a.Parent = parent
	a.Tags = nil
	a.Title = "RE: " + p.Title
	a.Index = int64(len(p.Replies))

	return m.db.Update(func(tx *bbolt.Tx) error {
		if err := tx.Bucket(bkPost).Put(idBytes(a.ID), a.marshal()); err != nil {
			return err
		}
		if err := tx.Bucket(bkPost).Put(idBytes(p.ID), p.marshal()); err != nil {
			return err
		}
		if err := m.appendUserInboxChain(tx, a.Author, a.ID, a.ID); err != nil {
			return err
		}
		if a.Author != p.Author { // Insert the notify of this reply to parent's author's inbox
			return m.appendUserInboxChain(tx, p.Author, newID(), a.ID)
		}
		return nil
	})
}

func (m *Manager) GetArticle(id int64) (*Article, error) {
	a := &Article{}
	return a, m.db.View(func(tx *bbolt.Tx) error {
		return a.unmarshal(tx.Bucket(bkPost).Get(idBytes(id)))
	})
}

func (m *Manager) UpdateArticle(a *Article, oldtags []string, del bool) error {
	return m.db.Update(func(tx *bbolt.Tx) error {
		idbuf := idBytes(a.ID)
		mustget := func(a, b []byte) *bbolt.Bucket {
			bk, _ := tx.Bucket(a).CreateBucketIfNotExists(b)
			return bk
		}

		if del {
			if err := tx.Bucket(bkPost).Delete(idbuf); err != nil {
				return err
			}
			if err := mustget(bkAuthor, []byte(a.Author)).Delete(idbuf); err != nil {
				return err
			}
			if err := m.deleteTags(tx, a.ID, a.Tags...); err != nil {
				return err
			}
		} else {
			newtags := append([]string{}, a.Tags...)

		DIFF:
			for i := len(oldtags) - 1; i > 0; i-- {
				for j, t := range newtags {
					if t == oldtags[i] {
						newtags = append(newtags[:j], newtags[j+1:]...)
						oldtags = append(oldtags[:i], oldtags[i+1:]...)
						continue DIFF
					}
				}
			}

			if err := m.deleteTags(tx, a.ID, oldtags...); err != nil {
				return err
			}
			if err := m.updateTags(tx, a.ID, newtags...); err != nil {
				return err
			}
			if err := tx.Bucket(bkPost).Put(idbuf, a.marshal()); err != nil {
				return err
			}
		}
		return nil
	})
}

func (m *Manager) Close() {
	m.closed = true
	m.db.Close()
}

func (m *Manager) GetHomePage() template.HTML {
	return template.HTML(m.home)
}

func (m *Manager) SetHomePage(buf string) {
	m.db.Update(func(tx *bbolt.Tx) error { return tx.Bucket(bkPost).Put([]byte("home"), []byte(buf)) })
	m.home = buf
}
