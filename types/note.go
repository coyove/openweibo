package types

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/coyove/sdss/contrib/clock"
	"github.com/coyove/sdss/contrib/simple"
	"github.com/gogo/protobuf/proto"
	"github.com/pierrec/lz4/v4"
)

type Note struct {
	Id              uint64   `protobuf:"fixed64,1,opt"`
	Title           string   `protobuf:"bytes,2,opt"`
	ReviewTitle     string   `protobuf:"bytes,3,opt"`
	Content         string   `protobuf:"bytes,4,opt"`
	ReviewContent   string   `protobuf:"bytes,5,opt"`
	ParentIds       []uint64 `protobuf:"fixed64,6,rep"`
	ReviewParentIds []uint64 `protobuf:"fixed64,19,rep"`
	Creator         string   `protobuf:"bytes,7,opt"`
	Modifier        string   `protobuf:"bytes,8,opt"`
	Reviewer        string   `protobuf:"bytes,9,opt"`
	PendingReview   bool     `protobuf:"varint,10,opt"`
	Lock            bool     `protobuf:"varint,11,opt"`
	CreateUnix      int64    `protobuf:"fixed64,12,opt"`
	UpdateUnix      int64    `protobuf:"fixed64,13,opt"`
	Image           string   `protobuf:"bytes,14,opt"`
	ReviewImage     string   `protobuf:"bytes,15,opt"`
	ChildrenCount   int64    `protobuf:"varint,16,opt"`
	TouchCount      int64    `protobuf:"varint,18,opt"`
}

func (t *Note) Reset() { *t = Note{} }

func (t *Note) ProtoMessage() {}

func (t *Note) String() string { return proto.CompactTextString(t) }

func (t *Note) Clone() *Note { return UnmarshalNoteBinary(t.MarshalBinary()) }

func (t *Note) IdStr() string { return clock.Base40Encode(t.Id) }

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
		return "ns:id:" + clock.Base40Encode(t.Id)
	}
	return FullEscape(t.Title)
}

func (t *Note) QueryTitle() string {
	if t.Title == "" {
		return "ns:id:" + clock.Base40Encode(t.Id)
	}
	return url.QueryEscape(t.Title)
}

func (t *Note) TitleDisplay() string {
	tt := t.Title
	if tt == "" {
		return "ns:id:" + t.IdStr()
	}
	return tt
}

func (t *Note) HTMLTitleDisplay() string {
	tt := SafeHTML(t.Title)
	if tt == "" {
		return fmt.Sprintf("<span style='font-style:italic'>ns:id:%s</span>", t.IdStr())
	}
	return tt
}

func (t *Note) FileExt() string {
	return strings.ToLower(filepath.Ext(t.Image))
}

func (t *Note) FileSizeString() string {
	return fmt.Sprintf("%.2fM", float64(t.FileSize())/1024/1024)
}

func (t *Note) FileSize() int {
	if t.Image == "" {
		return 0
	}
	idx1, idx2 := strings.IndexByte(t.Image, '('), strings.IndexByte(t.Image, ')')
	if idx1 > 0 && idx2 > idx1 {
		a, _ := strconv.Atoi(t.Image[idx1+1 : idx2])
		return a
	}
	return 0
}

func (t *Note) IsImage() bool {
	switch strings.ToLower(filepath.Ext(t.Image)) {
	case ".jpg", ".jpeg", ".jpe", ".jif", ".jfif", ".jfi",
		".png", ".gif", ".webp", ".tiff", ".tif", ".bmp",
		".dib", ".heif", ".heic", ".jp2", ".j2k", ".jpf",
		".jpx", ".jpm", ".mj2":
		return true
	}
	return false
}

func (t *Note) ClearReviewStatus() {
	t.PendingReview = false
	t.ReviewTitle, t.ReviewContent, t.ReviewImage, t.ReviewParentIds = "", "", "", nil
}

func (t *Note) ReviewDataNotChanged() bool {
	return t.ReviewTitle == t.Title &&
		t.ReviewContent == t.Content &&
		t.ReviewImage == t.Image &&
		simple.Uint64.Equal(t.ReviewParentIds, t.ParentIds)
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

func UpdateNoteBytes(p []byte, f func(n *Note)) []byte {
	t := &Note{}
	if err := proto.Unmarshal(p, t); err != nil {
		panic(err)
	}
	f(t)
	return t.MarshalBinary()
}

type Record struct {
	Id         uint64 `protobuf:"fixed64,1,opt"`
	Action     int64  `protobuf:"varint,2,opt"`
	NoteBytes  []byte `protobuf:"bytes,3,opt"`
	Modifier   string `protobuf:"bytes,4,opt"`
	ModifierIP string `protobuf:"bytes,5,opt"`
	CreateUnix int64  `protobuf:"fixed64,6,opt"`
	RejectMsg  string `protobuf:"bytes,7,opt"`
}

func (t *Record) Reset() { *t = Record{} }

func (t *Record) ProtoMessage() {}

func (t *Record) String() string { return proto.CompactTextString(t) }

func (t *Record) SetNote(n *Note) {
	out := &bytes.Buffer{}
	x := n.MarshalBinary()
	w := lz4.NewWriter(out)
	w.Write(x)
	w.Close()
	t.NoteBytes = out.Bytes()
}

func (t *Record) Note() *Note {
	rd := lz4.NewReader(bytes.NewReader(t.NoteBytes))
	buf, _ := ioutil.ReadAll(rd)
	return UnmarshalNoteBinary(buf)
}

func (t *Record) MarshalBinary() []byte {
	buf, _ := proto.Marshal(t)
	return buf
}

func (t *Record) BytesDiff() string {
	n := t.Note()
	if t.Action == 'c' {
		if len(n.Content) == 0 {
			return ""
		}
		return "+" + strconv.Itoa(len(n.Content))
	}
	a, b := len(n.Content), len(n.ReviewContent)
	if a == b {
		return ""
	}
	if b > a {
		return "+" + strconv.Itoa(b-a)
	}
	return strconv.Itoa(b - a)
}

func (t *Record) ActionName() string {
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

func UnmarshalRecordBinary(p []byte) *Record {
	t := &Record{}
	proto.Unmarshal(p, t)
	return t
}

type Image struct {
	Id         uint64 `protobuf:"fixed64,1,opt"`
	NoteId     uint64 `protobuf:"fixed64,2,opt"`
	UploadUnix uint64 `protobuf:"fixed64,3,opt"`
	CreateUnix int64  `protobuf:"fixed64,4,opt"`
}

func (t *Image) Reset() { *t = Image{} }

func (t *Image) ProtoMessage() {}

func (t *Image) String() string { return proto.CompactTextString(t) }

func UnmarshalImageBinary(p []byte) *Image {
	t := &Image{}
	proto.Unmarshal(p, t)
	return t
}

func (t *Image) MarshalBinary() []byte {
	buf, _ := proto.Marshal(t)
	return buf
}
