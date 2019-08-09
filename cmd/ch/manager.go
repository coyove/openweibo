package main

import (
	"fmt"
	"math"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
)

type Manager struct {
	name     string
	articles *mgo.Collection
	session  *mgo.Session
	closed   bool
}

func NewManager(name, uri string) (*Manager, error) {
	client, err := mgo.Dial(uri)
	if err != nil {
		return nil, err
	}
	m := &Manager{
		name:     name,
		session:  client,
		articles: client.DB(name).C("articles"),
	}

	m.articles.DropCollection()
	m.articles.EnsureIndex(mgo.Index{Key: []string{"reply_time", "create_time", "author", "tags", "parent"}})
	if err := m.articles.EnsureIndex(mgo.Index{Key: []string{"$text:title"}}); err != nil {
		m.session.Close()
		return nil, err
	}
	return m, nil
}

type findby bson.M

func ByTags(tag ...string) findby {
	return findby(bson.M{"tags": bson.M{"$all": tag}})
}

func ByTitle(title string) findby {
	return findby(bson.M{"$text": bson.M{"$search": title}})
}

func ByAuthor(author uint64) findby {
	return findby(bson.M{"author": author})
}

func ByNone() findby {
	return findby(bson.M{})
}

func (m *Manager) FindBack(filter findby, cursor int64, n int) ([]*Article, bool, error) {
	filter["parent"] = bson.M{"$exists": false}
	filter["reply_time"] = bson.M{"$gt": cursor}

	q := m.articles.Find(bson.M(filter)).Sort("reply_time").Limit(n + 1)
	a := []*Article{}
	if err := q.All(&a); err != nil {
		return nil, false, err
	}

	more := len(a) == n+1
	if more {
		a = a[:n]
	}

	for left, right := 0, len(a)-1; left < right; left, right = left+1, right-1 {
		a[left], a[right] = a[right], a[left]
	}
	return a, more, nil
}

func (m *Manager) Find(filter findby, cursor int64, n int) ([]*Article, bool, error) {
	if cursor == 0 {
		cursor = math.MaxInt64
	}
	filter["reply_time"] = bson.M{"$lt": cursor}
	filter["parent"] = bson.M{"$exists": false}

	q := m.articles.Find(bson.M(filter)).Sort("-reply_time").Limit(n + 1)
	a := []*Article{}
	if err := q.All(&a); err != nil {
		return nil, false, err
	}
	more := len(a) == n+1
	if more {
		a = a[:n]
	}
	return a, more, nil
}

func (m *Manager) FindRepliesBack(parent bson.ObjectId, cursor int64, n int) ([]*Article, bool, error) {
	if cursor == -1 {
		cursor = math.MaxInt64
	}

	q := m.articles.Find(bson.M{
		"create_time": bson.M{"$lt": cursor},
		"parent":      parent,
	}).Sort("-create_time").Limit(n + 1)

	a := []*Article{}
	if err := q.All(&a); err != nil {
		return nil, false, err
	}

	more := len(a) == n+1
	if more {
		a = a[:n]
	}

	for left, right := 0, len(a)-1; left < right; left, right = left+1, right-1 {
		a[left], a[right] = a[right], a[left]
	}
	return a, more, nil
}

func (m *Manager) FindReplies(parent bson.ObjectId, cursor int64, n int) ([]*Article, bool, error) {
	q := m.articles.Find(bson.M{
		"create_time": bson.M{"$gt": cursor},
		"parent":      parent,
	}).Sort("create_time").Limit(n + 1)

	a := []*Article{}
	if err := q.All(&a); err != nil {
		return nil, false, err
	}
	more := len(a) == n+1
	if more {
		a = a[:n]
	}
	return a, more, nil
}

func (m *Manager) PostArticle(a *Article) error {
	return m.articles.Insert(a)
}

func (m *Manager) PostReply(parent bson.ObjectId, a *Article) error {
	p, err := m.GetArticle(parent)
	if err != nil {
		return err
	}
	replyTime := time.Now().UnixNano() / 1e3
	if p.Locked {
		return fmt.Errorf("locked parent")
	}
	if p.Announcement {
		replyTime = math.MaxInt64 - 1
	}
	a.Parent = p.ID
	if err := m.articles.Insert(a); err != nil {
		return err
	}
	return m.articles.UpdateId(parent, bson.M{
		"$set": bson.M{"reply_time": replyTime},
		"$inc": bson.M{"replies_count": 1},
	})
}

func (m *Manager) GetArticle(id bson.ObjectId) (*Article, error) {
	a := []*Article{}
	if err := m.articles.FindId(id).All(&a); err != nil {
		return nil, err
	}
	if len(a) != 1 {
		return nil, fmt.Errorf("%s not found", objectIDToDisplayID(id))
	}
	return a[0], nil
}

func (m *Manager) UpdateArticle(id bson.ObjectId, del bool, title, content string, tags []string) error {
	if del {
		return m.articles.RemoveId(id)
	}
	return m.articles.UpdateId(id, bson.M{
		"$set": bson.M{
			"title":   title,
			"content": content,
			"tags":    tags,
		},
	})
}

func (m *Manager) AnnounceArticle(id bson.ObjectId) error {
	return m.articles.UpdateId(id, bson.M{
		"$set": bson.M{
			"announcement": true,
			"create_time":  math.MaxInt64 - 1,
			"reply_time":   math.MaxInt64 - 1,
		},
	})
}

func (m *Manager) LockArticle(id bson.ObjectId, v bool) error {
	return m.articles.UpdateId(id, bson.M{
		"$set": bson.M{
			"locked": v,
		},
	})
}

func (m *Manager) Close() {
	m.closed = true
	m.session.Close()
}
