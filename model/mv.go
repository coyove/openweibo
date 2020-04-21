package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/coyove/iis/common"
)

var (
	ErrNotExisted                = errors.New("article not existed")
	Dummy                        = User{_IsYou: true}
	DalIsFollowing               func(string, string) bool
	DalIsBlocking                func(string, string) bool
	DalIsFollowingWithAcceptance func(string, *User) (bool, bool)
)

type Cmd string

const (
	CmdNone            Cmd = ""
	CmdInboxReply          = "inbox-reply"
	CmdInboxMention        = "inbox-mention"
	CmdInboxLike           = "inbox-like"
	CmdInboxFwAccepted     = "inbox-fw-accepted"
	CmdFollow              = "follow"
	CmdFollowed            = "followed"
	CmdBlock               = "block"
	CmdLike                = "like"

	DeletionMarker = "[[b19b8759-391b-460a-beb0-16f5f334c34f]]"
)

const (
	ReplyLockNobody byte = 1 + iota
	ReplyLockFollowingsCan
	ReplyLockFollowingsMentionsCan
	ReplyLockFollowingsFollowersCan
)

type Article struct {
	ID            string            `json:"id"`
	Replies       int               `json:"rs,omitempty"`
	Likes         int32             `json:"like,omitempty"`
	ReplyLockMode byte              `json:"lm,omitempty"`
	NSFW          bool              `json:"nsfw,omitempty"`
	Content       string            `json:"content,omitempty"`
	Media         string            `json:"M,omitempty"`
	Author        string            `json:"author,omitempty"`
	IP            string            `json:"ip,omitempty"`
	CreateTime    time.Time         `json:"create,omitempty"`
	Parent        string            `json:"P,omitempty"`
	ReplyChain    string            `json:"Rc,omitempty"`
	NextReplyID   string            `json:"R,omitempty"`
	NextMediaID   string            `json:"MN,omitempty"`
	NextID        string            `json:"N,omitempty"`
	EOC           string            `json:"EO,omitempty"`
	Cmd           Cmd               `json:"K,omitempty"`
	Extras        map[string]string `json:"X,omitempty"`
	ReferID       string            `json:"ref,omitempty"`
	History       string            `json:"his,omitempty"`

	_StickOnTop bool
}

func (a *Article) SetStickOnTop(v bool) {
	a._StickOnTop = v
}

func (a *Article) StickOnTop() bool {
	return a._StickOnTop
}

func (a *Article) ContentHTML() template.HTML {
	if a.Content == DeletionMarker {
		a.Extras = nil
		return "<span class=deleted></span>"
	}
	return template.HTML(common.SanText(a.Content))
}

func (a *Article) PickNextID(media bool) string {
	if media {
		return a.NextMediaID
	}
	return a.NextID
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
	Email        string `json:"e"`
	Avatar       uint32 `json:"av"`
	CustomName   string `json:"cn"`
	Followers    int32  `json:"F"`
	Followings   int32  `json:"f"`
	Unread       int32  `json:"ur"`
	DataIP       string `json:"sip"`
	TSignup      uint32 `json:"st"`
	TLogin       uint32 `json:"lt"`
	Banned       bool   `json:"ban,omitempty"`
	Kimochi      byte   `json:"kmc,omitempty"`

	_IsFollowing            bool
	_IsFollowingNotAccepted bool
	_IsFollowed             bool
	_IsBlocking             bool
	_IsYou                  bool
	_IsInvalid              bool
	_ShowList               byte
	_Settings               UserSettings
}

func (u User) Marshal() []byte {
	b, _ := json.Marshal(u)
	return b
}

func (u User) DisplayName() string {
	marker := "@"
	if strings.HasPrefix(u.ID, "#") {
		marker = "#"
		u.ID = u.ID[1:]
	}

	if u.CustomName == "" {
		return marker + u.ID
	}
	return u.CustomName + " (" + marker + u.ID + ")"
}

func (u User) IsFollowing() bool { return u._IsFollowing }

func (u User) IsFollowingNotAccepted() bool { return u._IsFollowingNotAccepted }

func (u User) IsFollowed() bool { return u._IsFollowed }

func (u User) IsBlocking() bool { return u._IsBlocking }

func (u User) IsYou() bool { return u._IsYou }

func (u User) IsInvalid() bool { return u._IsInvalid }

func (u *User) SetInvalid() *User { u._IsInvalid = true; return u }

func (u User) ShowList() byte { return u._ShowList }

func (u User) Settings() UserSettings { return u._Settings }

func (u *User) Buildup(you *User) {
	following, accepted := DalIsFollowingWithAcceptance(you.ID, u)
	u._IsYou = you.ID == u.ID
	if u._IsYou {
		return
	}
	u._IsFollowing = following
	u._IsFollowingNotAccepted = following && !accepted
	u._IsFollowed = DalIsFollowing(u.ID, you.ID)
	u._IsBlocking = DalIsBlocking(you.ID, u.ID)
}

func (u *User) SetShowList(t byte) { u._ShowList = t }

func (u *User) SetSettings(s UserSettings) { u._Settings = s }

func (u User) JSON() string {
	b, _ := json.MarshalIndent(u, "", "")
	b = bytes.TrimLeft(b, " \r\n\t{")
	b = bytes.TrimRight(b, " \r\n\t}")
	return string(b)
}

func (u User) Signup() time.Time { return time.Unix(int64(u.TSignup), 0) }

func (u User) Login() time.Time { return time.Unix(int64(u.TLogin), 0) }

func (u User) IsMod() bool { return u.Role == "mod" || u.ID == common.Cfg.AdminName }

func (u User) IsAdmin() bool { return u.Role == "admin" || u.ID == common.Cfg.AdminName }

func (u User) IDHash() (hash uint64) {
	for _, r := range u.ID {
		hash = hash*31 + uint64(r)
	}
	return
}

func UnmarshalUser(b []byte) (*User, error) {
	a := &User{}
	err := json.Unmarshal(b, a)
	if a.ID == "" {
		return nil, fmt.Errorf("failed to unmarshal: %q", b)
	}

	common.AddUserToSearch(a.ID)
	return a, err
}

type UserSettings struct {
	AutoNSFW                   bool      `json:"autonsfw,omitempty"`
	FoldImages                 bool      `json:"foldi,omitempty"`
	OnlyMyFollowingsCanFollow  bool      `json:"mffm,omitempty"`
	OnlyMyFollowingsCanMention bool      `json:"mfcm,omitempty"`
	Description                string    `json:"desc,omitempty"`
	FollowerNeedsAcceptance    time.Time `json:"pdfollow,omitempty"`
}

func (u UserSettings) DoFollowerNeedsAcceptance() bool {
	return u.FollowerNeedsAcceptance != (time.Time{}) && !u.FollowerNeedsAcceptance.IsZero()
}

func (u UserSettings) Marshal() []byte {
	p, _ := json.Marshal(u)
	return p
}

func (u UserSettings) DescHTML() template.HTML {
	return template.HTML(common.SanText(u.Description))
}

// Always return a valid struct, though sometimes being empty
func UnmarshalUserSettings(b []byte) UserSettings {
	a := UserSettings{}
	json.Unmarshal(b, &a)
	return a
}
