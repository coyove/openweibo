package main

import (
	"encoding/binary"
	"log"
	"math"
	"strconv"
	"strings"
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
	m.articles.EnsureIndex(mgo.Index{Key: []string{"reply_time", "author", "tags", "parent"}})
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

func (m *Manager) ByTags(tag ...string) findby {
	return findby(bson.M{"tags": bson.M{"$all": tag}})
}

func (m *Manager) ByTitle(title string) findby {
	return findby(bson.M{"$text": bson.M{"$search": title}})
}

func (m *Manager) ByParent(parent bson.ObjectId) findby {
	return findby(bson.M{"parent": parent})
}

func (m *Manager) ByNone() findby {
	return findby(bson.M{})
}

func (m *Manager) FindBack(filter findby, sort sortby, cursor int64, n int) ([]*Article, error) {
	filter[string(sort)] = bson.M{"$gt": cursor}
	if _, ok := filter["parent"]; !ok {
		filter["parent"] = bson.M{"$exists": false}
	}

	q := m.articles.Find(bson.M(filter)).Sort(string(sort)).Limit(n)
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
	if cursor == 0 {
		cursor = math.MaxInt64
	}
	filter[string(sort)] = bson.M{"$lt": cursor}
	if _, ok := filter["parent"]; !ok {
		filter["parent"] = bson.M{"$exists": false}
	}

	q := m.articles.Find(bson.M(filter)).Sort("-" + string(sort)).Limit(n)
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
		"$inc": bson.M{"replies": 1},
	})
}

func (m *Manager) Close() {
	m.session.Close()
}

type Article struct {
	ID         bson.ObjectId `bson:"_id"`
	Parent     bson.ObjectId `bson:"parent,omitempty"`
	Replies    int64         `bson:"replies_count"`
	Title      string        `bson:"title"`
	Content    string        `bson:"content"`
	Author     string        `bson:"author"`
	Images     []string      `bson:"images"`
	Tags       []string      `bson:"tags"`
	CreateTime int64         `bson:"create_time"`
	ReplyTime  int64         `bson:"reply_time"`
}

func NewArticle(title, content, author string, images []string, tags []string) *Article {
	return &Article{
		ID:         bson.NewObjectId(),
		Title:      title,
		Content:    content,
		Author:     author,
		Images:     images,
		Tags:       tags,
		CreateTime: time.Now().UnixNano() / 1e3,
		ReplyTime:  time.Now().UnixNano() / 1e3,
	}
}

func (a *Article) DisplayID() string {
	return objectIDToDisplayID(a.ID)
}

func (a *Article) DisplayParentID() string {
	return objectIDToDisplayID(a.Parent)
}

func (a *Article) String() string {
	log.Println(a.ID == displayIDToObejctID(a.DisplayID()))
	return a.DisplayID() + "-" + a.Title + "-" + strconv.FormatInt(a.ReplyTime, 10) + "-" + a.DisplayParentID()
}

func objectIDToDisplayID(id bson.ObjectId) string {
	if id == "" {
		return ""
	}
	x := []byte(id)
	for i := 0; i < 11; i++ {
		x[i] += x[11] * x[11]
	}
	e3 := func(b []byte) string {
		return strconv.FormatUint(uint64(b[0])<<40+uint64(b[1])<<32+
			uint64(b[2])<<24+uint64(b[3])<<16+
			uint64(b[4])<<8+uint64(b[5])<<0, 8)
	}
	return e3(x) + "." + e3(x[6:])
}

func displayIDToObejctID(id string) bson.ObjectId {
	if id == "" {
		return ""
	}
	idx := strings.Index(id, ".")
	if idx == -1 {
		return ""
	}
	x := make([]byte, 16)
	v1, _ := strconv.ParseUint(id[:idx], 8, 64)
	v2, _ := strconv.ParseUint(id[idx+1:], 8, 64)
	binary.BigEndian.PutUint64(x[8:], v2)
	binary.BigEndian.PutUint64(x[2:], v1)
	x = x[4:]
	for i := 0; i < 11; i++ {
		x[i] -= x[11] * x[11]
	}
	return bson.ObjectId(x)
}
