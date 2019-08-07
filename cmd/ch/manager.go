package main

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
)

type Manager struct {
	name     string
	articles *mgo.Collection
	session  *mgo.Session
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

type sortby string

const (
	SortByReply  sortby = "reply_time"
	SortByCreate sortby = "create_time"
)

func ByTags(tag ...string) findby {
	return findby(bson.M{"tags": bson.M{"$all": tag}})
}

func ByTitle(title string) findby {
	return findby(bson.M{"$text": bson.M{"$search": title}})
}

func ByAuthor(author uint64) findby {
	return findby(bson.M{"author": author})
}

func ByParent(parent bson.ObjectId) findby {
	return findby(bson.M{"parent": parent})
}

func ByNone() findby {
	return findby(bson.M{})
}

func (m *Manager) FindBack(filter findby, sort sortby, cursor int64, n int) ([]*Article, error) {
	dir := ""
	if _, ok := filter["parent"]; !ok {
		// Find articles
		filter["parent"] = bson.M{"$exists": false}
		filter[string(sort)] = bson.M{"$gt": cursor}
	} else {
		// Find replies of an article
		if cursor == -1 {
			cursor = math.MaxInt64
		}
		dir = "-"
		filter[string(sort)] = bson.M{"$lt": cursor}
	}

	q := m.articles.Find(bson.M(filter)).Sort(dir + string(sort)).Limit(n)
	a := []*Article{}
	if err := q.All(&a); err != nil {
		return nil, err
	}
	for left, right := 0, len(a)-1; left < right; left, right = left+1, right-1 {
		a[left], a[right] = a[right], a[left]
	}
	return a, nil
}

func (m *Manager) Find(filter findby, sort sortby, cursor int64, n int) ([]*Article, error) {
	dir := "-"

	if _, ok := filter["parent"]; !ok {
		// Find articles
		if cursor == 0 {
			cursor = math.MaxInt64
		}
		filter[string(sort)] = bson.M{"$lt": cursor}
		filter["parent"] = bson.M{"$exists": false}
	} else {
		// Find replies of an article
		dir = ""
		filter[string(sort)] = bson.M{"$gt": cursor}
	}

	log.Println(filter)

	q := m.articles.Find(bson.M(filter)).Sort(dir + string(sort)).Limit(n)
	a := []*Article{}
	if err := q.All(&a); err != nil {
		return nil, err
	}
	return a, nil
}

func (m *Manager) PostArticle(a *Article) error {
	return m.articles.Insert(a)
}

func (m *Manager) PostReply(parent bson.ObjectId, a *Article) error {
	a.Parent = parent
	if err := m.articles.Insert(a); err != nil {
		return err
	}
	return m.articles.UpdateId(parent, bson.M{
		"$set": bson.M{"reply_time": time.Now().UnixNano() / 1e3},
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

func (m *Manager) Close() {
	m.session.Close()
}
