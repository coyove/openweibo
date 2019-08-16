package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"html/template"
	"math/rand"
	"strconv"
	"sync/atomic"
	"time"
)

type ArticlesView struct {
	Articles       []*Article
	ParentArticle  *Article
	Next, Prev     int64
	NoNext, NoPrev bool
	ShowIP         bool
	Type           string
	SearchTerm     string
	Title          string
}

type Article struct {
	ID         int64    `json:"id"`
	Parent     int64    `json:"p"`
	Replies    int64    `json:"rc"`
	Locked     bool     `json:"l"`
	Announce   bool     `json:"A"`
	Title      string   `json:"T"`
	Content    string   `json:"C"`
	Author     string   `json:"a"`
	IP         string   `json:"ip"`
	Image      string   `json:"i"`
	Tags       []string `json:"t"`
	CreateTime int64    `json:"c"`
	ReplyTime  int64    `json:"r"`
}

func newID() int64 {
	id := uint64(time.Now().Unix()) << 32
	id |= uint64(atomic.AddInt64(&m.counter, 1)) & 0xffff << 16
	id |= rand.Uint64() & 0xffff
	return int64(id)
}

func newBigID() int64 {
	id := uint64(newID())
	id |= 0x7fffffff00000000
	return int64(id)
}

func (m *Manager) NewArticle(title, content, author, ip string, image string, tags []string) *Article {
	return &Article{
		ID:         newID(),
		Title:      title,
		Content:    content,
		Author:     author,
		Image:      image,
		Tags:       tags,
		IP:         ip,
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

func (a *Article) CreateTimeString() string {
	return formatTime(a.CreateTime)
}

func (a *Article) ReplyTimeString() string {
	return formatTime(a.ReplyTime)
}

func (a *Article) ContentHTML() template.HTML {
	return template.HTML(sanText(a.Content))
}

func (a *Article) ImageURL() string {
	if a.Image != "" {
		return config.ImageDomain + "/i/" + a.Image
	}
	return ""
}

func (a *Article) String() string {
	return a.DisplayID() + "-" + a.Title + "-" + strconv.FormatInt(a.ReplyTime, 10) + "-" + a.DisplayParentID()
}

func (a *Article) Marshal() []byte {
	b, _ := json.Marshal(a)
	return b
}

func (a *Article) Unmarshal(b []byte) error {
	return json.Unmarshal(b, a)
}

func authorNameToHash(n string) string {
	h := hmac.New(sha1.New, []byte(config.Key))
	h.Write([]byte(n + config.Key))
	x := h.Sum(nil)
	if len(n) > 4 {
		n = n[:4]
	}
	return n + base64.URLEncoding.EncodeToString(x[:6])
}

func objectIDToDisplayID(id int64) string {
	return strconv.FormatInt(id, 8)
}

func displayIDToObejctID(id string) int64 {
	a, _ := strconv.ParseInt(id, 8, 64)
	return a
}

func idBytes(id int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}

func bytesID(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

func formatTime(t int64) string {
	x, now := time.Unix(0, t*1000), time.Now()
	if now.YearDay() == x.YearDay() && now.Year() == x.Year() {
		return x.Format("15:04:05")
	}
	if now.Year() == x.Year() {
		return x.Format("01-02 15:04:05")
	}
	return x.Format("2006-01-02 15:04:05")
}
