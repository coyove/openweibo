package types

import (
	"bytes"
	"io/ioutil"
	"net/url"
	"strconv"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/pierrec/lz4"
)

type Note struct {
	Id            uint64   `protobuf:"fixed64,1,opt" json:"I"`
	Title         string   `protobuf:"bytes,2,opt" json:"O"`
	ReviewTitle   string   `protobuf:"bytes,3,opt" json:"pn,omitempty"`
	Content       string   `protobuf:"bytes,4,opt" json:"D,omitempty"`
	ReviewContent string   `protobuf:"bytes,5,opt" json:"pd,omitempty"`
	ParentIds     []uint64 `protobuf:"fixed64,6,rep" json:"P,omitempty"`
	Creator       string   `protobuf:"bytes,7,opt" json:"U"`
	Modifier      string   `protobuf:"bytes,8,opt" json:"M,omitempty"`
	Reviewer      string   `protobuf:"bytes,9,opt" json:"R,omitempty"`
	PendingReview bool     `protobuf:"varint,10,opt" json:"pr,omitempty"`
	Lock          bool     `protobuf:"varint,11,opt" json:"L,omitempty"`
	CreateUnix    int64    `protobuf:"fixed64,12,opt" json:"C"`
	UpdateUnix    int64    `protobuf:"fixed64,13,opt" json:"u"`
	Image         string   `protobuf:"bytes,14,opt" json:"img,omitempty"`
	ReviewImage   string   `protobuf:"bytes,15,opt" json:"pimg,omitempty"`
	ChildrenCount int64    `protobuf:"varint,16,opt" json:"cn,omitempty"`
}

func (t *Note) Reset() { *t = Note{} }

func (t *Note) ProtoMessage() {}

func (t *Note) String() string { return proto.CompactTextString(t) }

func (t *Note) JoinParentIds() string {
	var tmp []string
	for _, id := range t.ParentIds {
		tmp = append(tmp, strconv.FormatUint(id, 10))
	}
	return strings.Join(tmp, ",")
}

func (t *Note) ContainsParents(ids []uint64) bool {
	s := 0
	for _, id := range ids {
		for _, p := range t.ParentIds {
			if p == id {
				s++
				break
			}
		}
	}
	return s > 0 && s == len(ids)
}

func FullEscape(v string) string {
	buf := &bytes.Buffer{}
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(v); i++ {
		x := v[i]
		buf.WriteByte('%')
		buf.WriteByte(hex[x>>4])
		buf.WriteByte(hex[x&0xf])
	}
	return buf.String()
}

func (t *Note) EscapedTitle() string {
	if t.Title == "" {
		return "ns:id:" + strconv.FormatUint(t.Id, 10)
	}
	return FullEscape(t.Title)
}

func (t *Note) QueryTitle() string {
	if t.Title == "" {
		return "ns:id:" + strconv.FormatUint(t.Id, 10)
	}
	return url.QueryEscape(t.Title)
}

func (t *Note) HTMLTitleDisplay() string {
	var tt string
	if t.PendingReview {
		tt = SafeHTML(t.ReviewTitle)
	} else {
		tt = SafeHTML(t.Title)
	}
	if tt == "" {
		return "<span class=untitled></span>"
	}
	return tt
}

func (t *Note) ClearReviewStatus() {
	t.PendingReview = false
	t.ReviewTitle, t.ReviewContent, t.ReviewImage = "", "", ""
}

func (t *Note) Valid() bool {
	return t != nil && t.Id > 0
}

func (t *Note) MarshalBinary() []byte {
	buf, err := proto.Marshal(t)
	if err != nil {
		panic(err)
	}
	return buf
}

func UnmarshalNoteBinary(p []byte) *Note {
	t := &Note{}
	if err := proto.Unmarshal(p, t); err != nil {
		panic(err)
	}
	return t
}

func IncrNoteChildrenCountBinary(p []byte, d int64) []byte {
	t := &Note{}
	if err := proto.Unmarshal(p, t); err != nil {
		panic(err)
	}
	t.ChildrenCount += d
	return t.MarshalBinary()
}

type NoteRecord struct {
	Id         uint64 `protobuf:"fixed64,1,opt"`
	Action     int64  `protobuf:"varint,2,opt"`
	NoteBytes  []byte `protobuf:"bytes,3,opt"`
	Modifier   string `protobuf:"bytes,4,opt"`
	ModifierIP string `protobuf:"bytes,5,opt"`
	CreateUnix int64  `protobuf:"fixed64,6,opt"`
}

func (t *NoteRecord) Reset() { *t = NoteRecord{} }

func (t *NoteRecord) ProtoMessage() {}

func (t *NoteRecord) String() string { return proto.CompactTextString(t) }

func (t *NoteRecord) SetNote(n *Note) {
	out := &bytes.Buffer{}
	x := n.MarshalBinary()
	w := lz4.NewWriter(out)
	w.Write(x)
	w.Close()
	t.NoteBytes = out.Bytes()
}

func (t *NoteRecord) Note() *Note {
	rd := lz4.NewReader(bytes.NewReader(t.NoteBytes))
	buf, _ := ioutil.ReadAll(rd)
	return UnmarshalNoteBinary(buf)
}

func (t *NoteRecord) MarshalBinary() []byte {
	buf, _ := proto.Marshal(t)
	return buf
}

func (t *NoteRecord) ActionName() string {
	switch t.Action {
	case 'c':
		return "create"
	case 'a':
		return "approve"
	case 'r':
		return "reject"
	case 'u':
		return "update"
	case 'd':
		return "delete"
	case 'L':
		return "lock"
	case 'U':
		return "unlock"
	}
	return "unknown"
}

func UnmarshalTagRecordBinary(p []byte) *NoteRecord {
	t := &NoteRecord{}
	proto.Unmarshal(p, t)
	return t
}
