package types

import (
	"encoding/json"
	"fmt"

	"github.com/coyove/sdss/contrib/clock"
	"github.com/gogo/protobuf/proto"
)

type Tag struct {
	Id            uint64   `protobuf:"fixed64,1,opt" json:"I"`
	Name          string   `protobuf:"bytes,2,opt" json:"O"`
	ReviewName    string   `protobuf:"bytes,3,opt" json:"pn,omitempty"`
	Desc          string   `protobuf:"bytes,4,opt" json:"D,omitempty"`
	ReviewDesc    string   `protobuf:"bytes,5,opt" json:"pd,omitempty"`
	ParentIds     []uint64 `protobuf:"fixed64,6,rep" json:"P,omitempty"`
	Creator       string   `protobuf:"bytes,7,opt" json:"U"`
	Modifier      string   `protobuf:"bytes,8,opt" json:"M,omitempty"`
	Reviewer      string   `protobuf:"bytes,9,opt" json:"R,omitempty"`
	PendingReview bool     `protobuf:"varint,10,opt" json:"pr,omitempty"`
	Lock          bool     `protobuf:"varint,11,opt" json:"L,omitempty"`
	CreateUnix    int64    `protobuf:"fixed64,12,opt" json:"C"`
	UpdateUnix    int64    `protobuf:"fixed64,13,opt" json:"u"`
}

func (t *Tag) MarshalBinary() []byte {
	buf, err := proto.Marshal(t)
	if err != nil {
		panic(err)
	}
	return buf
}

func (t *Tag) Reset() { *t = Tag{} }

func (t *Tag) ProtoMessage() {}

func (t *Tag) String() string { return proto.CompactTextString(t) }

func (t *Tag) Data() string {
	buf, _ := json.Marshal(t)
	return string(buf)
}

func (t *Tag) Valid() bool {
	return t != nil && t.Id > 0
}

func UnmarshalTagBinary(p []byte) *Tag {
	t := &Tag{}
	if err := proto.Unmarshal(p, t); err != nil {
		panic(err)
	}
	return t
}

type TagRecord struct {
	Id         uint64 `protobuf:"fixed64,1,opt" json:"I"`
	Action     int64  `protobuf:"varint,2,opt" json:"A"`
	Tag        *Tag   `protobuf:"bytes,3,opt" json:"T"`
	Modifier   string `protobuf:"bytes,4,opt" json:"M"`
	ModifierIP string `protobuf:"bytes,5,opt" json:"ip"`
	CreateUnix int64  `protobuf:"fixed64,6,opt" json:"C"`
}

func (t *TagRecord) Reset() { *t = TagRecord{} }

func (t *TagRecord) ProtoMessage() {}

func (t *TagRecord) String() string { return proto.CompactTextString(t) }

func (t *TagRecord) MarshalBinary() []byte {
	buf, _ := proto.Marshal(t)
	return buf
}

func UnmarshalTagRecordBinary(p []byte) *TagRecord {
	t := &TagRecord{}
	proto.Unmarshal(p, t)
	return t
}

type Document struct {
	Id      string `json:"I"`
	Content string `json:"C"`
}

func (doc Document) PartKey() string {
	ts := doc.CreateTime()
	return fmt.Sprintf("doc%d", ts>>16)
}

func (doc *Document) MarshalBinary() []byte {
	buf, _ := json.Marshal(doc)
	return buf
}

func (doc *Document) CreateTime() int64 {
	ts, _ := clock.ParseIdStrUnix(doc.Id)
	return ts
}

func (doc *Document) String() string {
	return fmt.Sprintf("%d(%s): %q", doc.CreateTime(), doc.Id, doc.Content)
}

func StrHash(s string) uint64 {
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211
	var hash uint64 = offset64
	for i := 0; i < len(s); i++ {
		hash *= prime64
		hash ^= uint64(s[i])
	}
	return uint64(hash)
}
