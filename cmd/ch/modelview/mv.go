package mv

import (
	"fmt"
	"html/template"

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
	ReplyView     struct {
		UUID      string
		Challenge string
		ShowReply bool
		RAuthor   string
		RContent  string
		EError    string
	}
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
	return FormatTime(a.CreateTime, sec)
}

func (a *Article) ReplyTimeString(sec bool) string {
	return FormatTime(a.ReplyTime, sec)
}

func (a *Article) ContentHTML() template.HTML {
	return template.HTML(sanText(a.Content))
}

func (a *Article) ContentAbstract() string {
	return SoftTrunc(a.Content, 64)
}

func (a *Article) MarshalA() []byte {
	b, _ := proto.Marshal(a)
	return b
}

func (a *Article) UnmarshalA(b []byte) error {
	err := proto.Unmarshal(b, a)
	if a.ID == nil {
		return fmt.Errorf("failed to unmarshal")
	}
	return err
}
