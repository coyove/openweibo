package types

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/coyove/sdss/contrib/clock"
)

type Tag struct {
	Id            uint64   `json:"I"`
	Name          string   `json:"O"`
	ReviewName    string   `json:"pn,omitempty"`
	Desc          string   `json:"D,omitempty"`
	ReviewDesc    string   `json:"pd,omitempty"`
	ParentIds     []uint64 `json:"P,omitempty"`
	Creator       string   `json:"U"`
	Modifier      string   `json:"M,omitempty"`
	Reviewer      string   `json:"R,omitempty"`
	PendingReview bool     `json:"pr,omitempty"`
	Lock          bool     `json:"L,omitempty"`
	CreateUnix    int64    `json:"C"`
	UpdateUnix    int64    `json:"u"`
}

func (t *Tag) MarshalBinary() []byte {
	buf, _ := json.Marshal(t)
	return buf
}

func (t *Tag) Data() string {
	out := &bytes.Buffer{}
	w := json.NewEncoder(out)
	w.SetEscapeHTML(true)
	w.Encode(t)
	return out.String()
}

func (t *Tag) String() string {
	return string(t.MarshalBinary())
}

func (t *Tag) Valid() bool {
	return t != nil && t.Id > 0
}

func UnmarshalTagBinary(p []byte) *Tag {
	t := &Tag{}
	json.Unmarshal(p, t)
	if t.UpdateUnix == 0 {
		t.UpdateUnix = t.CreateUnix
	}
	return t
}

type TagRecord struct {
	Id         uint64 `json:"I"`
	Action     string `json:"A"`
	Modifier   string `json:"M"`
	ModifierIP string `json:"ip"`
	From       string `json:"F"`
	To         string `json:"T"`
	CreateUnix int64  `json:"C"`
}

type TagRecordDiff struct {
	TagRecord
	TagId uint64
	Diffs [][3]interface{}
}

func (t *TagRecord) MarshalBinary() []byte {
	buf, _ := json.Marshal(t)
	return buf
}

func UnmarshalTagRecordBinary(p []byte) *TagRecord {
	t := &TagRecord{}
	json.Unmarshal(p, t)
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
