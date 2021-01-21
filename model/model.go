package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/coyove/iis/common/geoip"

	"github.com/coyove/iis/common"
)

var (
	ErrNotExisted                = errors.New("article not existed")
	Dummy                        = User{_IsYou: true, ID: "dummy"}
	DalIsFollowing               func(string, string) bool
	DalIsBlocking                func(string, string) bool
	DalIsFollowingWithAcceptance func(string, *User) (bool, bool)
)

type Cmd string

const (
	CmdNone            Cmd = ""
	CmdInboxReply          = "inbox-reply"
	CmdInboxMention        = "inbox-mention"
	CmdInboxFwApply        = "inbox-fw-apply"
	CmdInboxFwAccepted     = "inbox-fw-accepted"
	CmdFollow              = "follow"
	CmdFollowed            = "followed"
	CmdBlock               = "block"
	CmdLike                = "like"          // raw cmd article
	CmdInboxLike           = "inbox-like"    // notification shown in inbox
	CmdTimelineLike        = "timeline-like" // notification shown in timeline

	DeletionMarker = "[[b19b8759-391b-460a-beb0-16f5f334c34f]]"
)

const (
	ReplyLockNobody byte = 1 + iota
	ReplyLockFollowingsCan
	ReplyLockFollowingsMentionsCan
	ReplyLockFollowingsFollowersCan

	PostOptionNoMasterTimeline byte = 1
	PostOptionNoTimeline            = 2
	PostOptionNoSearch              = 4
)

type Article struct {
	ID            string            `json:"id"`
	Replies       int               `json:"rs,omitempty"`      // how many replies
	Likes         int32             `json:"like,omitempty"`    // how many likes
	ReplyLockMode byte              `json:"lm,omitempty"`      // reply lock
	PostOptions   byte              `json:"po,omitempty"`      // post options
	Asc           byte              `json:"asc,omitempty"`     // replies order by asc
	NSFW          bool              `json:"nsfw,omitempty"`    // NSFW
	Anonymous     bool              `json:"anon,omitempty"`    // anonymous
	Content       string            `json:"content,omitempty"` // content
	Media         string            `json:"M,omitempty"`       // media string
	Author        string            `json:"author,omitempty"`  // author ID
	IP            string            `json:"ip,omitempty"`      // IP
	CreateTime    time.Time         `json:"create,omitempty"`  // create time
	Parent        string            `json:"P,omitempty"`       // reply to ID  +----------------- EOC -------------.
	ReplyChain    string            `json:"Rc,omitempty"`      //              +-------- NextMediaID ---------.     `v
	NextReplyID   string            `json:"R,omitempty"`       //      +------ p -- NextID -> p2 -- NextID -> p3 ... pLast
	NextMediaID   string            `json:"MN,omitempty"`      //      |       | <- ReplyChain
	NextID        string            `json:"N,omitempty"`       //  ReplyEOC   r1
	EOC           string            `json:"EO,omitempty"`      //      |       | <- NextID
	ReplyEOC      string            `json:"REO,omitempty"`     //      +----> r2 -- ReplyChain -> r2_1 -- NextID -> r2_2 ...
	Cmd           Cmd               `json:"K,omitempty"`       // command
	Extras        map[string]string `json:"X,omitempty"`       // extras
	ReferID       string            `json:"ref,omitempty"`     // refer ID (to another article)
	History       string            `json:"his,omitempty"`     // operation history

	T_StickOnTop bool `json:"-"`
}

func (a *Article) IsDeleted() bool {
	return a.Content == DeletionMarker
}

func (a *Article) ContentHTML() template.HTML {
	if a.IsDeleted() {
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
	indexArticle(a)
	return a, err
}

type User struct {
	ID                    string
	Session               string
	Role                  string
	PasswordHash          []byte
	Email                 string `json:"e"`
	Avatar                uint32 `json:"av"`
	CustomName            string `json:"cn"`
	Followers             int32  `json:"F"`
	Followings            int32  `json:"f"`
	Unread                int32  `json:"ur"`
	DataIP                string `json:"sip"`
	TSignup               uint32 `json:"st"`
	TLogin                uint32 `json:"lt"`
	Banned                bool   `json:"ban,omitempty"`
	Kimochi               byte   `json:"kmc,omitempty"`
	FollowApply           uint32 `json:"fap,omitempty"`
	ExpandNSFWImages      int    `json:"an,omitempty"`
	FoldAllImages         int    `json:"foldi,omitempty"`
	NotifyFollowerActOnly int    `json:"nfao,omitempty"`
	HideLikes             int    `json:"hfav,omitempty"`
	HideLocation          int    `json:"hl,omitempty"`
	Description           string `json:"desc,omitempty"`
	APIToken              string `json:"api,omitempty"`

	_IsFollowing            bool
	_IsFollowingNotAccepted bool
	_IsFollowed             bool
	_IsBlocking             bool
	_IsYou                  bool
	_IsInvalid              bool
	_IsAnon                 bool
	_IsAPI                  bool
	_ShowList               byte
}

func (u User) Marshal() []byte {
	b, _ := json.Marshal(u)
	return b
}

func (u User) AvatarURL() string {
	if u.Avatar > 0 && common.Cfg.MediaDomain != "" {
		return fmt.Sprintf("%s/%016x@%s?q=%d", common.Cfg.MediaDomain, u.IDHash(), u.ID, u.Avatar)
	}
	return fmt.Sprintf("/avatar/%s?q=%d", u.ID, u.Avatar)
}

func (u User) KimochiURL() string {
	return fmt.Sprintf("/s/emoji/emoji%d.png", u.Kimochi)
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

func (u User) RecentIPLocation() string {
	if u.HideLocation == 1 {
		return ""
	}
	for _, part := range strings.Split(u.DataIP, ",") {
		part = strings.Trim(strings.TrimSpace(part), "{}")
		if len(part) == 0 {
			continue
		}
		var data = strings.Split(part, "/")
		_, loc := geoip.LookupIP(data[0])
		return loc
	}
	return ""
}

func (u User) IsFollowing() bool { return u._IsFollowing }

func (u User) IsFollowingNotAccepted() bool { return u._IsFollowingNotAccepted }

func (u User) IsFollowed() bool { return u._IsFollowed }

func (u User) IsBlocking() bool { return u._IsBlocking }

func (u User) IsYou() bool { return u._IsYou }

func (u User) IsInvalid() bool { return u._IsInvalid }

func (u User) IsAnon() bool { return u._IsAnon }

func (u User) IsAPI() bool { return u._IsAPI }

func (u *User) SetInvalid() *User { u._IsInvalid = true; return u }

func (u *User) SetIsAnon(v bool) *User { u._IsAnon = v; return u }

func (u *User) SetIsAPI(v bool) *User { u._IsAPI = v; return u }

func (u User) ShowList() byte { return u._ShowList }

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

func (u User) FollowApplyPivotTime() time.Time {
	return time.Unix(int64(u.FollowApply), 0)
}

func (u *User) DescHTML() template.HTML {
	return template.HTML(common.SanText(u.Description))
}

func UnmarshalUser(b []byte) (*User, error) {
	a := &User{}
	err := json.Unmarshal(b, a)
	if a.ID == "" {
		return nil, fmt.Errorf("failed to unmarshal: %q", b)
	}
	IndexUser(a)
	return a, err
}
