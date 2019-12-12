package mv

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
)

var ErrNotExisted = errors.New("article not existed")

type Cmd string

const (
	CmdNone    Cmd = ""
	CmdReply       = "inbox-reply"
	CmdMention     = "inbox-mention"
	CmdFollow      = "follow"
	CmdBlock       = "block"

	DeletionMarker = "[[b19b8759-391b-460a-beb0-16f5f334c34f]]"
)

type Article struct {
	ID          string            `json:"id"`
	Replies     int               `json:"rs,omitempty"`
	Upvotes     int               `json:"up,omitempty"`
	Locked      bool              `json:"lock,omitempty"`
	Highlighted bool              `json:"hl,omitempty"`
	Image       string            `json:"img,omitempty"`
	Title       string            `json:"title,omitempty"`
	Content     string            `json:"content"`
	Author      string            `json:"author"`
	IP          string            `json:"ip"`
	Category    string            `json:"cat,omitempty"`
	CreateTime  time.Time         `json:"create,omitempty"`
	Parent      string            `json:"P"`
	ReplyChain  string            `json:"Rc"`
	NextReplyID string            `json:"R"`
	NextID      string            `json:"N"`
	Cmd         Cmd               `json:"K"`
	Extras      map[string]string `json:"X"`
}

func (a *Article) ContentHTML() template.HTML {
	if a.Content == DeletionMarker {
		return "<span class=deleted></span>"
	}
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
	BuildIndex(a)
	return a, err
}

type User struct {
	ID           string
	Session      string
	Role         string
	PasswordHash []byte
	Email        string `json:"e"`
	Avatar       string `json:"a"`
	TotalPosts   int    `json:"tp"`
	Followers    int    `json:"F"`
	Followings   int    `json:"f"`
	//Blockings      int       `json:"b"`
	FollowingChain string    `json:"FC"`
	BlockingChain  string    `json:"BC"`
	Unread         int       `json:"ur"`
	Signup         time.Time `json:"st"`
	SignupIP       string    `json:"sip"`
	Login          time.Time `json:"lt"`
	LoginIP        string    `json:"lip"`
	Banned         bool      `json:"ban"`
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
