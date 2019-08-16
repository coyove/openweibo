package main

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/etcd-io/bbolt"
)

var errNoBucket = errors.New("")

type Manager struct {
	db      *bbolt.DB
	mu      sync.Mutex
	counter int64
	closed  bool
}

func NewManager(path string) (*Manager, error) {
	db, err := bbolt.Open(path, 0700, nil)
	if err != nil {
		return nil, err
	}

	if err := db.Update(func(tx *bbolt.Tx) error {
		for _, bk := range bkNames {
			if _, err = tx.CreateBucketIfNotExists(bk); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &Manager{
		db:      db,
		counter: rand.Int63(),
	}, nil
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

func ByNotify(a string) findby {
	return findby{
		bkName:  bkNotify,
		bkName2: []byte(a),
	}
}

func ByIP(ip string) findby {
	return findby{
		bkName:  bkIP,
		bkName2: []byte(ip),
	}
}

func ByTimeline() findby {
	return findby{
		bkName: bkTimeline,
	}
}

func (m *Manager) Find(dir byte, filter findby, cursor int64, n int) ([]*Article, bool, error) {
	var (
		more bool
		a    []*Article
		err  = m.db.View(func(tx *bbolt.Tx) error {
			main := tx.Bucket(bkPost)
			bk := tx.Bucket(filter.bkName)
			if filter.bkName2 != nil {
				bk = bk.Bucket(filter.bkName2)
			}
			if bk == nil {
				return nil
			}

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

			for _, r := range res {
				p := &Article{}
				if bytes.Equal(filter.bkName, bkNotify) {
					if p.Unmarshal(main.Get(r[1])) == nil {
						a = append(a, p)
					}
				} else {
					if p.Unmarshal(main.Get(r[0])) == nil {
						a = append(a, p)
					}
				}
			}
			return nil
		})
	)
	return a, more, err
}

func (m *Manager) FindTags(cursor string, n int) ([]string, int) {
	var a []string
	m.db.View(func(tx *bbolt.Tx) error {
		res, _ := ScanBucketAsc(tx.Bucket(bkTag), []byte(cursor), n, false)
		for _, r := range res {
			a = append(a, string(r[0]))
		}
		n = tx.Bucket(bkTag).Stats().BucketN
		return nil
	})
	return a, n
}

func (m *Manager) FindReplies(dir byte, parent int64, cursor int64, n int) ([]*Article, bool, error) {
	var (
		more bool
		a    []*Article
		err  = m.db.View(func(tx *bbolt.Tx) error {
			main := tx.Bucket(bkPost)

			bk := tx.Bucket(bkReply).Bucket(idBytes(parent))
			if bk == nil {
				return errNoBucket
			}

			var res [][2][]byte
			var next []byte

			if dir == 'a' {
				res, next = ScanBucketAsc(bk, idBytes(cursor), n, false)
			} else {
				if cursor == -1 {
					cursor = 0
				}
				res, next = ScanBucketDesc(bk, idBytes(cursor), n, true)
			}

			more = next != nil
			for _, r := range res {
				p := &Article{}
				if p.Unmarshal(main.Get(r[0])) == nil {
					a = append(a, p)
				}
			}
			return nil
		})
	)
	if err == errNoBucket {
		err = nil
	}
	return a, more, err
}

func (m *Manager) insertID(tx *bbolt.Tx, filter findby, id int64) error {
	var err error
	bk := tx.Bucket(filter.bkName)
	if filter.bkName2 != nil {
		bk, err = bk.CreateBucketIfNotExists(filter.bkName2)
	}
	if err != nil {
		return err
	}
	return bk.Put(idBytes(id), []byte{})
}

func (m *Manager) PostArticle(a *Article) error {
	return m.db.Update(func(tx *bbolt.Tx) error {
		idbuf := idBytes(a.ID)
		if err := tx.Bucket(bkPost).Put(idbuf, a.Marshal()); err != nil {
			return err
		}
		if err := m.insertID(tx, ByAuthor(a.Author), a.ID); err != nil {
			return err
		}
		if err := m.insertID(tx, ByIP(a.IP), a.ID); err != nil {
			return err
		}
		for _, tag := range a.Tags {
			if err := m.insertID(tx, ByTag(tag), a.ID); err != nil {
				return err
			}
		}
		if err := m.insertID(tx, ByTimeline(), a.ID); err != nil {
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

	p.ReplyTime = time.Now().UnixNano() / 1e3
	p.Replies++
	a.Parent = parent
	a.Title = "RE: " + p.Title

	return m.db.Update(func(tx *bbolt.Tx) error {
		if err := tx.Bucket(bkPost).Put(idBytes(a.ID), a.Marshal()); err != nil {
			return err
		}
		if err := tx.Bucket(bkPost).Put(idBytes(p.ID), p.Marshal()); err != nil {
			return err
		}
		if err := m.insertID(tx, ByAuthor(a.Author), a.ID); err != nil {
			return err
		}
		if err := m.insertID(tx, ByIP(a.IP), a.ID); err != nil {
			return err
		}
		if err := m.insertID(tx, findby{bkName: bkReply, bkName2: idBytes(p.ID)}, a.ID); err != nil {
			return err
		}
		// Insert the notify of this reply to parent's author's inbox
		bk, err := tx.Bucket(bkNotify).CreateBucketIfNotExists([]byte(p.Author))
		if err != nil {
			return err
		}
		if err := bk.Put(idBytes(newID()), idBytes(a.ID)); err != nil {
			return err
		}
		if bk.Stats().KeyN > config.InboxSize {
			k, _ := bk.Cursor().First()
			if err := bk.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func (m *Manager) GetArticle(id int64) (*Article, error) {
	a := &Article{}
	return a, m.db.View(func(tx *bbolt.Tx) error {
		return a.Unmarshal(tx.Bucket(bkPost).Get(idBytes(id)))
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
			if err := tx.Bucket(bkTimeline).Delete(idbuf); err != nil {
				return err
			}
			if err := mustget(bkAuthor, []byte(a.Author)).Delete(idbuf); err != nil {
				return err
			}
			if err := mustget(bkIP, []byte(a.IP)).Delete(idbuf); err != nil {
				return err
			}
			if err := mustget(bkNotify, []byte(a.Author)).Delete(idbuf); err != nil {
				return err
			}
			if err := mustget(bkReply, idBytes(a.Parent)).Delete(idbuf); err != nil {
				return err
			}
			for _, tag := range a.Tags {
				if err := mustget(bkTag, []byte(tag)).Delete(idbuf); err != nil {
					return err
				}
			}
		} else {
			newtags := make([]string, len(a.Tags))
			copy(newtags, a.Tags)

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

			for _, tag := range oldtags {
				if err := mustget(bkTag, []byte(tag)).Delete(idbuf); err != nil {
					return err
				}
			}

			for _, tag := range newtags {
				if err := m.insertID(tx, ByTag(tag), a.ID); err != nil {
					return err
				}
			}

			if err := tx.Bucket(bkPost).Put(idbuf, a.Marshal()); err != nil {
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
