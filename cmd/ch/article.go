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
	TotalCount     int
	NoNext, NoPrev bool
	ShowIP         bool
	Type           string
	SearchTerm     string
	Title          string
	ReplyView      interface{}
}

type Article struct {
	ID          int64    `json:"id"`
	Index       int64    `json:"idx"`
	Parent      int64    `json:"p"`
	Replies     int64    `json:"rc"`
	Locked      bool     `json:"l,omitempty"`
	Highlighted bool     `json:"h,omitempty"`
	Announce    bool     `json:"A,omitempty"`
	Title       string   `json:"T"`
	Content     string   `json:"C"`
	Author      string   `json:"a"`
	IP          string   `json:"ip"`
	Image       string   `json:"i"`
	Tags        []string `json:"t"`
	Views       int64    `json:"v"`
	CreateTime  int64    `json:"c"`
	ReplyTime   int64    `json:"r"`

	SearchTerm string `json:"-"`
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

func (a *Article) CreateTimeString(sec bool) string {
	return formatTime(a.CreateTime, sec)
}

func (a *Article) ReplyTimeString(sec bool) string {
	return formatTime(a.ReplyTime, sec)
}

func (a *Article) ContentHTML() template.HTML {
	return template.HTML(sanText(a.Content))
}

func (a *Article) ContentAbstract() string {
	return softTrunc(a.Content, 64)
}

func (a *Article) ImageURL() string {
	if a.Image != "" {
		return "//" + config.ImageDomain + "/i/" + a.Image
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
	var n0 string
	if len(n) >= 4 {
		n0 = n[:4]
		for i := 0; i < len(n0); i++ {
			if n0[i] > 127 {
				n0 = n0[:i]
				break
			}
		}
		n = n[4:]
	}

	h := hmac.New(sha1.New, []byte(config.Key))
	h.Write([]byte(n + config.Key))
	x := h.Sum(nil)
	return n0 + base64.URLEncoding.EncodeToString(x[:6])
}

func objectIDToDisplayID(id int64) string {
	var (
		sum    uint32 = 0
		delta  uint32 = 0x9E3779B9
		v0, v1        = uint32(uint64(id) >> 32), uint32(uint64(id))
	)
	for i := 0; i < 64; i++ {
		v0 += (((v1 << 4) ^ (v1 >> 5)) + v1) ^ sum
		sum += delta
		v1 += (((v0 << 4) ^ (v0 >> 5)) + v0) ^ sum
	}
	return strconv.FormatUint(uint64(v0)<<32|uint64(v1), 36)
}

func displayIDToObejctID(id string) int64 {
	xxx, _ := strconv.ParseUint(id, 36, 64)

	var (
		v0           = uint32(xxx >> 32)
		v1           = uint32(xxx)
		delta uint32 = 0x9E3779B9
		sum   uint32 = delta * 64
	)
	for i := 0; i < 64; i++ {
		v1 -= (((v0 << 4) ^ (v0 >> 5)) + v0) ^ sum
		sum -= delta
		v0 -= (((v1 << 4) ^ (v1 >> 5)) + v1) ^ sum
	}
	return int64(uint64(v0)<<32 + uint64(v1))
}

func idBytes(id int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}

func formatTime(t int64, sec bool) string {
	x, now := time.Unix(0, t*1000), time.Now()
	if now.YearDay() == x.YearDay() && now.Year() == x.Year() {
		if !sec {
			return x.Format("15:04")
		}
		return x.Format("15:04:05")
	}
	if now.Year() == x.Year() {
		if !sec {
			return x.Format("Jan 02")
		}
		return x.Format("Jan 02 15:04")
	}
	if !sec {
		return x.Format("Jan 02, 2006")
	}
	return x.Format("Jan 02, 2006 15:04")
}
