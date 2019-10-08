package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/id"
	"github.com/gogo/protobuf/proto"
)

type ArticlesTimelineView struct {
	Tags         []string
	Articles     []*Article
	Next         string
	Prev         string
	SearchTerm   string
	ShowAbstract bool
}

type ArticleRepliesView struct {
	Tags          []string
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
	Timeline    []byte `protobuf:"bytes,12,opt"`
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

func (m *Manager) NewPost(title, content, author, ip string, cat string) *Article {
	return &Article{
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
	return id.BytesString(a.ID)
}

func (a *Article) DisplayTimeline() string {
	return id.BytesString(a.Timeline)
}

func (a *Article) DisplayParentID() string {
	return id.BytesString(a.Parent)
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
		n0 = strings.TrimLeft(n[:4], "#")
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
