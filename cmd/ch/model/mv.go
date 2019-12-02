package mv

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
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
		return nil, fmt.Errorf("failed to unmarshal: %q", p)
	}
	return t, err
}

type Article struct {
	ID         string `json:"id"`
	TimelineID string `json:"tlid"`
	Replies    int    `json:"rs"`
	//Views       int       `json:"vs"`
	Locked      bool      `json:"lock,omitempty"`
	Highlighted bool      `json:"hl,omitempty"`
	Image       string    `json:"img,omitempty"`
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

func (a *Article) Parent() (string, string) {
	p, tp := ident.ParseID(a.ID).RIndexParent()
	return p.String(), tp.String()
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
		return nil, fmt.Errorf("failed to unmarshal: %q", b)
	}
	return a, err
}

type User struct {
	ID           string
	Session      string
	Role         string
	PasswordHash []byte
	Email        string    `json:"e"`
	TotalPosts   int       `json:"tp"`
	Unread       int       `json:"ur"`
	Signup       time.Time `json:"st"`
	SignupIP     string    `json:"sip"`
	Login        time.Time `json:"lt"`
	LoginIP      string    `json:"lip"`
	Banned       bool      `json:"ban"`
}

func (u User) Marshal() []byte {
	b, _ := json.Marshal(u)
	return b
}

func (u User) IsMod() bool {
	return u.Role == "admin" || u.Role == "mod" || u.ID == config.Cfg.AdminName
}

func (u User) IsAdmin() bool {
	return u.Role == "admin" || u.ID == config.Cfg.AdminName
}

func UnmarshalUser(b []byte) (*User, error) {
	a := &User{}
	err := json.Unmarshal(b, a)
	if a.ID == "" {
		return nil, fmt.Errorf("failed to unmarshal: %q", b)
	}
	return a, err
}

func MakeUserToken(u *User) string {
	if u == nil {
		return ""
	}

	length := len(u.ID) + 1 + len(u.Session)
	length = (length + 7) / 8 * 8

	x := make([]byte, length)
	copy(x, u.Session)
	copy(x[len(u.Session)+1:], u.ID)

	for i := 0; i <= len(x)-16; i += 8 {
		config.Cfg.Blk.Encrypt(x[i:], x[i:])
	}
	return base64.StdEncoding.EncodeToString(x)
}
