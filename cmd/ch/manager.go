package main

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/etcd-io/bbolt"
)

var (
	errNoBucket = errors.New("")
	bkPost      = []byte("post")
	bkAuthorTag = []byte("authortag")
)

type Manager struct {
	db      *bbolt.DB
	mu      sync.Mutex
	counter int64
	closed  bool
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
		if _, err = tx.CreateBucketIfNotExists(bkPost); err != nil {
			return err
		}
		if _, err = tx.CreateBucketIfNotExists(bkAuthorTag); err != nil {
			return err
		}
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
	return findby{bkName: bkAuthorTag, bkName2: []byte("#" + tag)}
}

func ByAuthor(a string) findby {
	return findby{bkName: bkAuthorTag, bkName2: []byte(a)}
}

func ByTimeline() findby {
	return findby{bkName: bkPost}
}

func (m *Manager) FindPosts(dir byte, bkName, partitionKey []byte, cursor int64, n int) ([]*Article, bool, int, error) {
	var (
		more  bool
		a     []*Article
		count int
		err   = m.db.View(func(tx *bbolt.Tx) error {
			bk := tx.Bucket(bkName)
			if partitionKey != nil {
				bk = bk.Bucket(partitionKey)
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

			if bytes.Equal(bkName, bkPost) {
				a = mget(tx, true, res)
				for i := len(a) - 1; i >= 0; i-- {
					if a[i].Parent != 0 {
						a = append(a[:i], a[i+1:]...)
					}
				}
			} else {
				a = mget(tx, false, res)
			}
			return nil
		})
	)
	return a, more, count, err
}

func (m *Manager) TagsCount(tags ...string) map[string]int {
	ret := map[string]int{}
	m.db.View(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(bkAuthorTag)
		for _, tag := range tags {
			bk := bk.Bucket([]byte("#" + tag))
			if bk == nil {
				ret[tag] = 0
				continue
			}
			ret[tag] = bk.Stats().KeyN
		}
		return nil
	})
	return ret
}

func (m *Manager) insertTags(bk *bbolt.Bucket, id int64, tags ...string) error {
	for _, tag := range tags {
		if err := m.insertBKV(bk, "#"+tag, id, id, false); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) deleteTags(bk *bbolt.Bucket, id int64, tags ...string) error {
	for _, tag := range tags {
		if err := m.deleteBKV(bk, "#"+tag, id); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) insertBKV(bk *bbolt.Bucket, b string, k, v int64, limitSize bool) error {
	bk, err := bk.CreateBucketIfNotExists([]byte(b))
	if err != nil {
		return err
	}
	if err = bk.Put(idBytes(k), idBytes(v)); err != nil {
		return err
	}
	if bk.Stats().KeyN > config.InboxSize && limitSize {
		k, _ := bk.Cursor().First()
		return bk.Delete(k)
	}
	return nil
}

func (m *Manager) deleteBKV(bk *bbolt.Bucket, b string, k int64) error {
	bk2 := bk.Bucket([]byte(b))
	if bk2 == nil {
		return nil
	}
	return bk2.Delete(idBytes(k))
}

func (m *Manager) PostPost(a *Article) error {
	return m.db.Update(func(tx *bbolt.Tx) error {
		bk := tx.Bucket(bkPost)
		a.Index = int64(bk.Stats().KeyN + 1)
		if err := bk.Put(idBytes(a.ID), a.marshal()); err != nil {
			return err
		}

		bk = tx.Bucket(bkAuthorTag)
		if err := m.insertBKV(bk, a.Author, a.ID, a.ID, true); err != nil {
			return err
		}
		if err := m.insertTags(bk, a.ID, a.Tags...); err != nil {
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
		if err := m.insertBKV(tx.Bucket(bkAuthorTag), a.Author, a.ID, a.ID, true); err != nil {
			return err
		}
		if a.Author != p.Author { // Insert the notify of this reply to parent's author's inbox
			return m.insertBKV(tx.Bucket(bkAuthorTag), p.Author, newID(), a.ID, true)
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
		bk := tx.Bucket(bkAuthorTag)

		if del {
			if err := tx.Bucket(bkPost).Delete(idBytes(a.ID)); err != nil {
				return err
			}
			if err := m.deleteBKV(bk, a.Author, a.ID); err != nil {
				return err
			}
			if err := m.deleteTags(bk, a.ID, a.Tags...); err != nil {
				return err
			}
		} else {
			newtags := append([]string{}, a.Tags...)
			for i := len(oldtags) - 1; i > 0; i-- {
				for j, t := range newtags {
					if t == oldtags[i] {
						newtags = append(newtags[:j], newtags[j+1:]...)
						oldtags = append(oldtags[:i], oldtags[i+1:]...)
						break
					}
				}
			}
			if err := m.deleteTags(bk, a.ID, oldtags...); err != nil {
				return err
			}
			if err := m.insertTags(bk, a.ID, newtags...); err != nil {
				return err
			}
			if err := tx.Bucket(bkPost).Put(idBytes(a.ID), a.marshal()); err != nil {
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
