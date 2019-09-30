package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"html/template"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
)

type ArticlesTimelineView struct {
	Articles   []*Article
	Next       string
	Prev       string
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
	ReplyView     interface{}
}

type Article struct {
	ID          []byte `protobuf:"bytes,1,opt"`
	Index       int64  `protobuf:"varint,2,opt"`
	Parent      []byte `protobuf:"bytes,3,opt"`
	Replies     int64  `protobuf:"varint,4,opt"`
	Locked      bool   `protobuf:"varint,5,opt"`
	Highlighted bool   `protobuf:"varint,6,opt"`
	Announce    bool   `protobuf:"varint,7,opt"`
	Title       string `protobuf:"bytes,8,opt"`
	Content     string `protobuf:"bytes,9,opt"`
	Author      string `protobuf:"bytes,10,opt"`
	IP          string `protobuf:"bytes,11,opt"`
	Category    string `protobuf:"bytes,13,opt"`
	Views       int64  `protobuf:"varint,14,opt"`
	CreateTime  uint32 `protobuf:"fixed32,15,opt"`
	ReplyTime   uint32 `protobuf:"fixed32,16,opt"`

	// Transient
	NotFound  bool `protobuf:"varint,18,opt"`
	BeReplied bool `protobuf:"varint,19,opt"`
}

func (a *Article) Reset() { *a = Article{} }

func (a *Article) String() string { return proto.CompactTextString(a) }

func (a *Article) ProtoMessage() {}

// For normal posts
func newID() (id []byte) {
	id = make([]byte, 10)
	ctr := uint32(atomic.AddInt64(&m.counter, 1))

	// 1 + 1 + 33 + 24 + 5
	v := 1<<63 | uint64(time.Now().Unix())<<29 | uint64(ctr&0xffffff)<<5 | uint64(rand.Uint64()&0x1f)
	binary.BigEndian.PutUint64(id, v)
	binary.BigEndian.PutUint16(id[8:], uint16(rand.Uint64()))
	return
}

func newBigID() []byte {
	id := newID()
	id[0] |= 0x40
	return id
}

func newReplyID(parent []byte, index uint16, out []byte) (id []byte) {
	if out != nil {
		id = out
	} else {
		id = make([]byte, len(parent)+2)
	}
	copy(id, parent)
	binary.BigEndian.PutUint16(id[len(parent):], index)
	id[0] &= 0x7f
	return
}

func getReplyIndex(replyID []byte) uint16 {
	if len(replyID) < 12 {
		return 0
	}
	return binary.BigEndian.Uint16(replyID[len(replyID)-2:])
}

func (m *Manager) NewPost(title, content, author, ip string, cat string) *Article {
	return &Article{
		ID:         newID(),
		Title:      title,
		Content:    content,
		Author:     author,
		Category:   cat,
		IP:         ip,
		CreateTime: uint32(time.Now().Unix()),
		ReplyTime:  uint32(time.Now().Unix()),
	}
}

func (m *Manager) NewReply(content, author, ip string) *Article {
	return &Article{
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

func (a *Article) marshal() []byte {
	b, _ := proto.Marshal(a)
	return b
}

func (a *Article) unmarshal(b []byte) error {
	err := proto.Unmarshal(b, a)
	if a.ID == nil {
		return fmt.Errorf("failed to unmarshal")
	}
	return err
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

var idEncoding = base64.NewEncoding("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz~").WithPadding('-')

func objectIDToDisplayID(id []byte) string {
	return idEncoding.EncodeToString(id)
}

func displayIDToObjectID(id string) []byte {
	buf, _ := idEncoding.DecodeString(id)
	return buf
}

func idBytes(id []byte) []byte {
	return id
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
