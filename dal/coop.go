package dal

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/coyove/iis/common"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
)

type (
	UpdateUserRequest struct {
		ID                 string
		Signup             bool
		ToggleMod          *bool
		ToggleBan          *bool
		IncDecUnread       *bool
		IncDecFollowers    *bool
		IncDecFollowings   *bool
		Session            *string
		PasswordHash       *[]byte
		Email              *string
		CustomName         *string
		Avatar             *uint32
		Unread             *int32
		DataIP             *string
		TSignup            *uint32
		TLogin             *uint32
		Kimochi            *byte
		SettingAutoNSFW    *bool
		SettingFoldImages  *bool
		SettingDescription *string
		SettingMFFM        *bool
		SettingMFCM        *bool
		SettingFwAccept    *bool
	}

	UpdateArticleRequest struct {
		ID                string
		IncDecLikes       *bool
		IncDecReplies     *bool
		ClearNextID       *bool
		DeleteBy          *model.User
		ToggleNSFWBy      *model.User
		UpdateReplyLockBy *model.User
		UpdateReplyLock   *byte
	}

	UpdateArticleExtraRequest struct {
		ID            string
		SetExtraKey   string
		SetExtraValue string
	}

	InsertArticleRequest struct {
		ID              string
		AsReply         bool
		AsFollowingSlot bool
		NoLock          bool
		Article         model.Article
	}

	UpdateOrInsertCmdArticleRequest struct {
		ArticleID          string
		InsertUnderChainID string
		Cmd                string
		ToSubject          string
		CmdValue           bool
	}
)

func DoUpdateUser(rr *UpdateUserRequest) (model.User, error) {
	id := rr.ID
	common.LockKey(id)
	defer common.UnlockKey(id)

	if rr.SettingAutoNSFW != nil ||
		rr.SettingFoldImages != nil ||
		rr.SettingDescription != nil ||
		rr.SettingMFFM != nil ||
		rr.SettingMFCM != nil ||
		rr.SettingFwAccept != nil {

		sid := "u/" + id + "/settings"
		p, _ := m.db.Get(sid)
		u := model.UnmarshalUserSettings(p)

		setIfValid(&u.AutoNSFW, rr.SettingAutoNSFW)
		setIfValid(&u.FoldImages, rr.SettingFoldImages)
		setIfValid(&u.Description, rr.SettingDescription)
		setIfValid(&u.OnlyMyFollowingsCanFollow, rr.SettingMFFM)
		setIfValid(&u.OnlyMyFollowingsCanMention, rr.SettingMFCM)

		if rr.SettingFwAccept != nil {
			if *rr.SettingFwAccept {
				u.FollowerNeedsAcceptance = time.Now()
			} else {
				u.FollowerNeedsAcceptance = time.Time{}
			}
		}

		return model.User{}, m.db.Set(sid, u.Marshal())
	}

	u, err := GetUser(id)
	if err == model.ErrNotExisted && rr.Signup {
		u = &model.User{ID: id}
		err = nil
	}

	if err != nil {
		return model.User{}, err
	}

	if rr.Signup {
		if len(u.PasswordHash) != 0 {
			return model.User{}, fmt.Errorf("id/already-existed")
		}
	}

	if rr.ToggleMod != nil && *rr.ToggleMod {
		if u.IsAdmin() {
			return model.User{}, fmt.Errorf("promote/admin-really")
		}
		if u.Role == "mod" {
			u.Role = ""
		} else {
			u.Role = "mod"
		}
	}
	if rr.ToggleBan != nil && *rr.ToggleBan {
		if u.IsAdmin() {
			return model.User{}, fmt.Errorf("ban/mod-really")
		}
		u.Banned = !u.Banned
	}
	setIfValid(&u.Kimochi, rr.Kimochi)
	setIfValid(&u.Session, rr.Session)
	setIfValid(&u.PasswordHash, rr.PasswordHash)
	setIfValid(&u.Email, rr.Email)
	setIfValid(&u.Avatar, rr.Avatar)
	setIfValid(&u.CustomName, rr.CustomName)
	setIfValid(&u.Unread, rr.Unread)
	setIfValid(&u.DataIP, rr.DataIP)
	setIfValid(&u.TSignup, rr.TSignup)
	setIfValid(&u.TLogin, rr.TLogin)
	if rr.IncDecFollowers != nil {
		incdec(&u.Followers, nil, *rr.IncDecFollowers)
	}
	if rr.IncDecFollowings != nil {
		incdec(&u.Followings, nil, *rr.IncDecFollowings)
	}
	if rr.IncDecUnread != nil {
		incdec(&u.Unread, nil, *rr.IncDecUnread)
	}
	return *u, m.db.Set("u/"+u.ID, u.Marshal())
}

func DoUpdateArticle(rr *UpdateArticleRequest) (model.Article, error) {
	common.LockKey(rr.ID)
	defer common.UnlockKey(rr.ID)

	a, err := GetArticle(rr.ID)
	if err != nil {
		return model.Article{}, err
	}

	if rr.IncDecLikes != nil {
		incdec(&a.Likes, nil, *rr.IncDecLikes)
	}
	if rr.IncDecReplies != nil {
		incdec(nil, &a.Replies, *rr.IncDecReplies)
	}
	if rr.ClearNextID != nil && *rr.ClearNextID {
		a.NextID, a.NextMediaID = "", ""
	}
	now := func() string { return time.Now().Format(time.Stamp) }
	if rr.DeleteBy != nil {
		if rr.DeleteBy.ID != a.Author && !rr.DeleteBy.IsMod() {
			return model.Article{}, fmt.Errorf("user/not-allowed")
		}
		a.Content = model.DeletionMarker
		a.Media = ""
		a.History += fmt.Sprintf("{delete_by:%s:%v}", rr.DeleteBy.ID, now())

		if a.Parent != "" {
			go func() {
				_, err := DoUpdateArticle(&UpdateArticleRequest{ID: a.Parent, IncDecReplies: aws.Bool(false)})
				if err != nil {
					log.Println("Delete Article, failed to dec parent's reply count:", err)
				}
			}()
		}
	}
	if rr.ToggleNSFWBy != nil {
		if rr.ToggleNSFWBy.ID != a.Author && !rr.ToggleNSFWBy.IsMod() {
			return model.Article{}, fmt.Errorf("user/not-allowed")
		}
		a.NSFW = !a.NSFW
		a.History += fmt.Sprintf("{nsfw_by:%s:%v}", rr.ToggleNSFWBy.ID, now())
	}
	if rr.UpdateReplyLockBy != nil {
		if rr.UpdateReplyLockBy.ID != a.Author && !rr.UpdateReplyLockBy.IsMod() {
			return model.Article{}, fmt.Errorf("user/not-allowed")
		}
		a.ReplyLockMode = *rr.UpdateReplyLock
		a.History += fmt.Sprintf("{lock_by:%s:%v}", rr.UpdateReplyLockBy.ID, now())
	}

	return *a, m.db.Set(a.ID, a.Marshal())
}

func DoUpdateArticleExtra(rr *UpdateArticleExtraRequest) (string, error) {
	common.LockKey(rr.ID)
	defer common.UnlockKey(rr.ID)

	a, err := GetArticle(rr.ID)
	if err != nil {
		return "", err
	}

	oldExtraValue := a.Extras[rr.SetExtraKey]
	if a.Extras == nil {
		a.Extras = map[string]string{}
	}
	a.Extras[rr.SetExtraKey] = rr.SetExtraValue

	return oldExtraValue, m.db.Set(a.ID, a.Marshal())
}

func DoInsertArticle(r *InsertArticleRequest) (A model.Article, E error) {
	rootID := r.ID
	a := r.Article

	if a.CreateTime.IsZero() || a.CreateTime == (time.Time{}) {
		a.CreateTime = time.Now()
	}

	// UpdateOrInsert may call Insert internally, so NoLock will prevent deadlock
	if !r.NoLock {
		common.LockKey(rootID)
		defer common.UnlockKey(rootID)
	}

	root, err := GetArticle(rootID)
	if err != nil && err != model.ErrNotExisted {
		return model.Article{}, err
	}

	if err == model.ErrNotExisted {
		if r.AsReply {
			return model.Article{}, err
		}
		root = &model.Article{
			ID:         rootID,
			EOC:        a.ID,
			CreateTime: time.Now(),
			Extras:     map[string]string{},
		}
	}

	root.Replies++
	if root.Extras == nil { // Compatible with older records
		root.Extras = map[string]string{}
	}

	if a.StickOnTop() && ik.ParseID(rootID).Header() == ik.IDAuthor {
		root.Extras["stick_on_top"] = a.ID
	}

	if x, y := ik.ParseID(rootID), ik.ParseID(root.NextID); x.Header() == ik.IDAuthor && y.Valid() {
		now := time.Now()
		if now.Year() != y.Time().Year() || now.Month() != y.Time().Month() {
			// The very last article was made before this month, so we will create a checkpoint for long jmp
			go func() {
				a := &model.Article{
					ID:         makeCheckpointID(x.Tag(), y.Time()),
					ReferID:    root.NextID,
					CreateTime: time.Now(),
				}
				m.db.Set(a.ID, a.Marshal())
			}()
		}
	}

	switch {
	case r.AsReply:
		// The article is a reply to another feed, so insert it into root's reply chain
		a.NextReplyID, root.ReplyChain = root.ReplyChain, a.ID
	case r.AsFollowingSlot:
		// The article contains following info, it won't go into any chains
		// instead root's Extras will record the index.
		// 'index' is the last element of the article's ArticleID: u/<user_id>/follow/<index>
		root.Extras[lastElemInCompID(a.ID)] = "1"
	default:
		// The article is a normal feed, so insert it into root's main chain/media chain
		a.NextID, root.NextID = root.NextID, a.ID
		if a.Media != "" {
			a.NextMediaID, root.NextMediaID = root.NextMediaID, a.ID
		}
	}

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return model.Article{}, err
	}

	if err := m.db.Set(root.ID, root.Marshal()); err != nil {
		return model.Article{}, err
	}

	return a, nil
}

func DoUpdateOrInsertCmdArticle(rr *UpdateOrInsertCmdArticleRequest) (updated bool, err error) {
	common.LockKey(rr.ArticleID)
	defer common.UnlockKey(rr.ArticleID)

	a, err := GetArticle(rr.ArticleID)
	if err != nil {
		if err == model.ErrNotExisted {
			a := &model.Article{
				ID:  rr.ArticleID,
				Cmd: model.Cmd(rr.Cmd),
				Extras: map[string]string{
					"to":   rr.ToSubject,
					rr.Cmd: strconv.FormatBool(rr.CmdValue),
				},
				CreateTime: time.Now(),
			}

			if rr.Cmd == model.CmdLike {
				if toa, _ := GetArticle(rr.ToSubject); toa != nil {
					a.Media = toa.Media
				}
			}

			updated = true
			_, err = DoInsertArticle(&InsertArticleRequest{
				ID:      rr.InsertUnderChainID,
				Article: *a,
				NoLock:  coDistribute(rr.ArticleID) == coDistribute(rr.InsertUnderChainID), // deadlock fix
			})
		}
		return updated, err
	}

	updated = a.Extras[rr.Cmd] != strconv.FormatBool(rr.CmdValue)
	a.Extras[rr.Cmd] = strconv.FormatBool(rr.CmdValue)

	return updated, m.db.Set(a.ID, a.Marshal())
}

func coDistribute(id string) uint16 {
	return common.Hash16(id)
}
