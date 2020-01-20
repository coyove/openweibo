package dal

import (
	"fmt"
	"reflect"
	"time"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
)

var ErrInvalidRequest = fmt.Errorf("invalid request")

const (
	DoUpdateUser    string = "UpdateUser"
	DoUpdateArticle        = "UpdateArticle"
	DoInsertArticle        = "InsertArticle"
)

type Request struct {
	UpdateUserRequest *struct {
		ID               string
		ToggleMod        bool
		ToggleBan        bool
		Signup           bool
		IncUnread        bool
		IncDecFollowers  *bool
		IncDecFollowings *bool
		Session          *string
		PasswordHash     *[]byte
		Email            *string
		Avatar           *uint32
		CustomName       *string
		Unread           *int32
		DataIP           *string
		TSignup          *uint32
		TLogin           *uint32
		Kimochi          *byte

		SettingAutoNSFW    *bool
		SettingFoldImages  *bool
		SettingDescription *string

		Response struct {
			OldUser model.User
			User    model.User
		} `json:"-"`
	}
	UpdateArticleRequest *struct {
		ID            string
		SetExtraKey   *string
		SetExtraValue *string
		IncDecLikes   *bool
		DeleteBy      *model.User
		ToggleNSFWBy  *model.User
		Response      struct {
			OldExtraValue string
			Article       model.Article
		} `json:"-"`
	}
	InsertArticleRequest *struct {
		RootID  string
		AsReply bool
		Article model.Article
	}
	TestRequest *struct {
		A *int
	}
}

func NewRequest(name string, kv ...interface{}) *Request {
	if len(kv)%2 != 0 {
		panic("unmatched key-value pairs")
	}

	r := &Request{}
	rv := reflect.ValueOf(r).Elem()
	f := rv.FieldByName(name + "Request")
	if !f.IsValid() {
		panic(fmt.Sprintf("invalid request name: %v", name))
	}
	f.Set(reflect.New(f.Type().Elem()))

	f = f.Elem()
	for i := 0; i < len(kv); i += 2 {
		k, v := kv[i].(string), reflect.ValueOf(kv[i+1])
		rf := f.FieldByName(k)
		if !rf.IsValid() {
			panic(fmt.Sprintf("invalid field name: %v", k))
		}
		if rf.Kind() == reflect.Ptr {
			rf.Set(reflect.New(rf.Type().Elem()))
			rf.Elem().Set(v)
		} else {
			rf.Set(v)
		}
	}

	return r
}

func coUpdateUser(r *Request) error {
	rr := r.UpdateUserRequest
	if rr == nil {
		return ErrInvalidRequest
	}

	id := rr.ID
	common.LockKey(id)
	defer common.UnlockKey(id)

	updateUserSettings := func(id string, cb func(u *model.UserSettings) error) error {
		sid := "u/" + id + "/settings"
		p, _ := m.db.Get(sid)
		s := model.UnmarshalUserSettings(p)
		if err := cb(&s); err != nil {
			return err
		}
		return m.db.Set(sid, s.Marshal())
	}

	if rr.SettingAutoNSFW != nil {
		return updateUserSettings(rr.ID, func(u *model.UserSettings) error {
			u.AutoNSFW = *rr.SettingAutoNSFW
			return nil
		})
	}
	if rr.SettingFoldImages != nil {
		return updateUserSettings(rr.ID, func(u *model.UserSettings) error {
			u.FoldImages = *rr.SettingFoldImages
			return nil
		})
	}
	if rr.SettingDescription != nil {
		return updateUserSettings(rr.ID, func(u *model.UserSettings) error {
			u.Description = *rr.SettingDescription
			return nil
		})
	}

	u, err := GetUser(id)
	if err == model.ErrNotExisted && rr.Signup {
		u = &model.User{ID: id}
		err = nil
	}

	if err != nil {
		return err
	}

	if rr.Signup {
		if len(u.PasswordHash) != 0 {
			return fmt.Errorf("id/already-existed")
		}
	}

	rr.Response.OldUser = *u
	defer func() { rr.Response.User = *u }()

	if rr.ToggleMod {
		if u.IsAdmin() {
			return fmt.Errorf("promote/admin-really")
		}
		if u.Role == "mod" {
			u.Role = ""
		} else {
			u.Role = "mod"
		}
	}
	if rr.ToggleBan {
		if u.IsMod() {
			return fmt.Errorf("ban/mod-really")
		}
		u.Banned = !u.Banned
	}
	if rr.Kimochi != nil {
		u.Kimochi = *rr.Kimochi
	}
	if rr.Session != nil {
		u.Session = *rr.Session
	}
	if rr.PasswordHash != nil {
		u.PasswordHash = *rr.PasswordHash
	}
	if rr.Email != nil {
		u.Email = *rr.Email
	}
	if rr.Avatar != nil {
		u.Avatar = *rr.Avatar
	}
	if rr.CustomName != nil {
		u.CustomName = *rr.CustomName
	}
	if rr.IncDecFollowers != nil {
		if *rr.IncDecFollowers {
			u.Followers++
		} else {
			dec0(&u.Followers)
		}
	}
	if rr.IncDecFollowings != nil {
		if *rr.IncDecFollowings {
			u.Followings++
		} else {
			dec0(&u.Followings)
		}
	}
	if rr.Unread != nil {
		u.Unread = *rr.Unread
	}
	if rr.IncUnread {
		u.Unread++
	}
	if rr.DataIP != nil {
		u.DataIP = *rr.DataIP
	}
	if rr.TSignup != nil {
		u.TSignup = *rr.TSignup
	}
	if rr.TLogin != nil {
		u.TLogin = *rr.TLogin
	}
	if u.ID == "" {
		return nil
	}
	m.weakUsers.Delete(u.ID)
	return m.db.Set("u/"+u.ID, u.Marshal())
}

func coUpdateArticle(r *Request) error {
	rr := r.UpdateArticleRequest
	if rr == nil {
		return ErrInvalidRequest
	}

	common.LockKey(rr.ID)
	defer common.UnlockKey(rr.ID)

	a, err := GetArticle(rr.ID)
	if err != nil {
		return err
	}

	if rr.SetExtraKey != nil {
		rr.Response.OldExtraValue = a.Extras[*rr.SetExtraKey]
		a.Extras[*rr.SetExtraKey] = *rr.SetExtraValue
	}
	if rr.IncDecLikes != nil {
		if *rr.IncDecLikes {
			a.Likes++
		} else {
			dec0(&a.Likes)
		}
	}
	if rr.DeleteBy != nil {
		if rr.DeleteBy.ID != a.Author && !rr.DeleteBy.IsMod() {
			return fmt.Errorf("user/can-not-delete")
		}
		a.Content = model.DeletionMarker
		a.Media = ""
	}
	if rr.ToggleNSFWBy != nil {
		if rr.ToggleNSFWBy.ID != a.Author && !rr.ToggleNSFWBy.IsMod() {
			return fmt.Errorf("user/can-not-delete")
		}
		a.NSFW = !a.NSFW
	}
	rr.Response.Article = *a

	return m.db.Set(a.ID, a.Marshal())
}

func coInsertArticle(r *Request) error {
	if r.InsertArticleRequest == nil {
		return ErrInvalidRequest
	}

	rootID := r.InsertArticleRequest.RootID
	a := r.InsertArticleRequest.Article
	asReply := r.InsertArticleRequest.AsReply

	common.LockKey(rootID)
	defer common.UnlockKey(rootID)

	root, err := GetArticle(rootID)
	if err != nil && err != model.ErrNotExisted {
		return err
	}

	if err == model.ErrNotExisted {
		if asReply {
			return err
		}
		root = &model.Article{
			ID:         rootID,
			EOC:        a.ID,
			CreateTime: time.Now(),
		}
	}

	if x, y := ik.ParseID(rootID), ik.ParseID(root.NextID); x.Header() == ik.IDAuthor && y.Valid() {
		now := time.Now()
		if now.Year() != y.Time().Year() || now.Month() != y.Time().Month() {
			// The very last article was made before this month, so we will create a checkpoint for long jmp
			go func() {
				a := &model.Article{
					ID:         makeCheckpointID(x.Tag(), root.CreateTime),
					ReferID:    root.NextID,
					CreateTime: time.Now(),
				}
				m.db.Set(a.ID, a.Marshal())
			}()
		}
	}

	if !a.Alone {
		if asReply {
			a.NextReplyID, root.ReplyChain = root.ReplyChain, a.ID
		} else {
			a.NextID, root.NextID = root.NextID, a.ID
			if a.Media != "" {
				a.NextMediaID, root.NextMediaID = root.NextMediaID, a.ID
			}
		}
	}

	root.Replies++

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return err
	}

	if err := m.db.Set(root.ID, root.Marshal()); err != nil {
		return err
	}
	return nil
}

func Do(r *Request) error {
	switch {
	case r.UpdateUserRequest != nil:
		return coUpdateUser(r)
	case r.UpdateArticleRequest != nil:
		return coUpdateArticle(r)
	case r.InsertArticleRequest != nil:
		return coInsertArticle(r)
	default:
		return nil
	}
}
