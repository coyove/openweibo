package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"html/template"
	"math/rand"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
)

type ArticlesView struct {
	Articles   []*Article
	Next, Prev int64
	TotalCount int
	NoPrev     bool
	Type       string
	SearchTerm string
	Title      string
}

type ArticleRepliesView struct {
	Articles      []*Article
	ParentArticle *Article
	CurPage       int
	TotalPages    int
	Pages         []int
	ShowIP        bool
	Title         string
	ReplyView     interface{}
}

type Article struct {
	ID          int64    `protobuf:"fixed64,1,opt"`
	Index       int64    `protobuf:"varint,2,opt"`
	Parent      int64    `protobuf:"fixed64,3,opt"`
	Replies     []int64  `protobuf:"fixed64,4,rep"`
	Locked      bool     `protobuf:"varint,5,opt"`
	Highlighted bool     `protobuf:"varint,6,opt"`
	Announce    bool     `protobuf:"varint,7,opt"`
	Title       string   `protobuf:"bytes,8,opt"`
	Content     string   `protobuf:"bytes,9,opt"`
	Author      string   `protobuf:"bytes,10,opt"`
	IP          string   `protobuf:"bytes,11,opt"`
	Image       string   `protobuf:"bytes,12,opt"`
	Tags        []string `protobuf:"bytes,13,rep"`
	Views       int64    `protobuf:"varint,14,opt"`
	CreateTime  uint32   `protobuf:"fixed32,15,opt"`
	ReplyTime   uint32   `protobuf:"fixed32,16,opt"`

	// Transient
	SearchTerm string `protobuf:"bytes,17,opt"`
}

func (a *Article) Reset() { *a = Article{} }

func (a *Article) String() string { return proto.CompactTextString(a) }

func (a *Article) ProtoMessage() {}

// For normal posts
func newID() int64 {
	id := uint64(time.Now().Unix()) << 31
	id |= uint64(atomic.AddInt64(&m.counter, 1)) & 0xffff << 15
	id |= rand.Uint64() & 0x7fff
	return int64(id)
}

// For replies, they have small IDs so they will always be placed after normal posts
func newSmallID() int64 {
	id := uint64(time.Now().AddDate(-42, -7, -31).Unix()) << 31
	id |= uint64(atomic.AddInt64(&m.counter, 1)) & 0xffff << 15
	id |= rand.Uint64() & 0x7fff
	return int64(id)
}

func newBigID() int64 {
	id := uint64(newID())
	id |= 0x7fffffff80000000
	return int64(id)
}

func (m *Manager) NewPost(title, content, author, ip string, tags []string) *Article {
	return &Article{
		ID:         newID(),
		Title:      title,
		Content:    content,
		Author:     author,
		Tags:       tags,
		IP:         ip,
		CreateTime: uint32(time.Now().Unix()),
		ReplyTime:  uint32(time.Now().Unix()),
	}
}

func (m *Manager) NewReply(content, author, ip string) *Article {
	return &Article{
		ID:         newSmallID(),
		Content:    content,
		Author:     author,
		IP:         ip,
		CreateTime: uint32(time.Now().Unix()),
		ReplyTime:  uint32(time.Now().Unix()),
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

func (a *Article) marshal() []byte {
	b, _ := proto.Marshal(a)
	return b
}

func (a *Article) unmarshal(b []byte) error {
	return proto.Unmarshal(b, a)
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

func formatTime(t uint32, sec bool) string {
	x, now := time.Unix(int64(t), 0), time.Now()
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
