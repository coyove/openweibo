package mv

import (
	"encoding/json"
	"fmt"
	"html/template"
	"time"

	"github.com/coyove/iis/cmd/ch/ident"
)

type Timeline struct {
	ID   string
	Next string
	Ptr  string
}

func (t Timeline) Marshal() []byte {
	p, _ := json.Marshal(t)
	return p
}

func (t *Timeline) String() string {
	return fmt.Sprintf("<I:%s N:%s P:%s>", t.ID, t.Next, t.Ptr)
}

func UnmarshalTimeline(p []byte) (*Timeline, error) {
	t := &Timeline{}
	err := json.Unmarshal(p, t)
	if t.ID == "" {
		return nil, fmt.Errorf("failed to unmarshal")
	}
	return t, err
}

type Article struct {
	ID          string    `json:"id"`
	TimelineID  string    `json:"tlid"`
	Replies     int       `json:"rs"`
	Views       int       `json:"vs"`
	Locked      bool      `json:"lock,omitempty"`
	Highlighted bool      `json:"hl,omitempty"`
	Saged       bool      `json:"sage,omitempty"`
	Title       string    `json:"title,omitempty"`
	Content     string    `json:"content"`
	Author      string    `json:"author"`
	IP          string    `json:"ip"`
	Category    string    `json:"cat,omitempty"`
	CreateTime  time.Time `json:"create"`
	ReplyTime   time.Time `json:"reply"`
}

func (a *Article) Index() int {
	return int(ident.ParseID(a.ID).RIndex())
}

func (a *Article) Parent() string {
	i, _ := ident.ParseID(a.ID).RIndexParent()
	return i.String()
}

func (a *Article) ContentHTML() template.HTML {
	return template.HTML(sanText(a.Content))
}

func (a *Article) Marshal() []byte {
	b, _ := json.Marshal(a)
	return b
}

func UnmarshalArticle(b []byte) (*Article, error) {
	a := &Article{}
	err := json.Unmarshal(b, a)
	if a.ID == "" {
		return nil, fmt.Errorf("failed to unmarshal")
	}
	return a, err
}
