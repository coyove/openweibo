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
	CmdNone     Cmd = ""
	CmdReply        = "inbox-reply"
	CmdMention      = "inbox-mention"
	CmdFollow       = "follow"
	CmdFollowed     = "followed"
	CmdBlock        = "block"
	CmdLike         = "like"
	CmdVote         = "vote"

	DeletionMarker = "[[b19b8759-391b-460a-beb0-16f5f334c34f]]"
)

type Article struct {
	ID          string            `json:"id"`
	Replies     int               `json:"rs,omitempty"`
	Likes       int32             `json:"like,omitempty"`
	Locked      bool              `json:"lock,omitempty"`
	NSFW        bool              `json:"nsfw,omitempty"`
	Content     string            `json:"content"`
	Media       string            `json:"M,omitempty"`
	Author      string            `json:"author"`
	IP          string            `json:"ip"`
	CreateTime  time.Time         `json:"create,omitempty"`
	Parent      string            `json:"P"`
	ReplyChain  string            `json:"Rc"`
	NextReplyID string            `json:"R"`
	EOC         string            `json:"EO,omitempty"`
	NextID      string            `json:"N,omitempty"`
	Cmd         Cmd               `json:"K,omitempty"`
	Extras      map[string]string `json:"X,omitempty"`
	ReferID     string            `json:"ref,omitempty"`
}

func (a *Article) ContentHTML() template.HTML {
	if a.Content == DeletionMarker {
		a.Extras = nil
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
	return a, err
}

type User struct {
	ID                string
	Session           string
	Role              string
	PasswordHash      []byte
	Email             string `json:"e"`
	Avatar            string `json:"a"`
	TotalPosts        int32  `json:"tp"`
	Followers         int32  `json:"F"`
	Followings        int32  `json:"f"`
	Unread            int32  `json:"ur"`
	FollowingChain    string `json:"FC2,omitempty"`
	FollowerChain     string `json:"FrC,omitempty"`
	BlockingChain     string `json:"BC,omitempty"`
	LikeChain         string `json:"LC,omitempty"`
	DataIP            string `json:"sip"`
	TSignup           uint32 `json:"st"`
	TLogin            uint32 `json:"lt"`
	Banned            bool   `json:"ban,omitempty"`
	NoReplyInTimeline bool   `json:"nrit,omitempty"`
	NoPostInMaster    bool   `json:"npim,omitempty"`
	AutoNSFW          bool   `json:"autonsfw,omitempty"`
	FoldImages        bool   `json:"foldi,omitempty"`
	Kimochi           byte   `json:"kmc,omitempty"`
}

func (u User) Marshal() []byte {
	b, _ := json.Marshal(u)
	return b
}

func (u User) Signup() time.Time {
	return time.Unix(int64(u.TSignup), 0)
}

func (u User) Login() time.Time {
	return time.Unix(int64(u.TLogin), 0)
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

	AddUserToSearch(a.ID)
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
