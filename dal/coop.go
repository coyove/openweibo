package dal

import (
	"fmt"
	"reflect"
	"time"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
)

func callOrSet(ru reflect.Value, f ...interface{}) error {
	switch len(f) {
	case 1:
		out := reflect.ValueOf(f[0]).Call([]reflect.Value{ru})
		if len(out) == 1 {
			if err, ok := out[0].Interface().(error); ok && err != nil {
				return err
			}
		}
	case 2:
		ru.Elem().FieldByName(f[0].(string)).Set(reflect.ValueOf(f[1]))
	}
	return nil
}

func DoUpdateUser(id string, f ...interface{}) (*model.User, error) {
	common.LockKey(id)
	defer common.UnlockKey(id)
	u, err := GetUser(id)
	if err != nil {
		return nil, err
	}
	if err := callOrSet(reflect.ValueOf(u), f...); err != nil {
		return nil, err
	}
	if u.Followers < 0 {
		u.Followers = 0
	}
	if u.Followings < 0 {
		u.Followings = 0
	}
	if u.Unread < 0 {
		u.Unread = 0
	}
	return u, m.db.Set("u/"+u.ID, u.Marshal())
}

func DoSignUp(id string, passwordHash []byte, email, session, ip string) error {
	common.LockKey(id)
	defer common.UnlockKey(id)

	u, err := GetUser(id)
	if err != nil {
		if err == model.ErrNotExisted {
			goto CHECK
		}
		return err
	}

CHECK:
	if u != nil && len(u.PasswordHash) != 0 {
		return fmt.Errorf("e:duplicated_id")
	}

	u = &model.User{}
	u.ID = id
	u.Session = session
	u.PasswordHash = passwordHash
	u.Email = email
	u.TLogin = uint32(time.Now().Unix())
	u.TSignup = uint32(time.Now().Unix())
	u.DataIP = ip
	return m.db.Set("u/"+u.ID, u.Marshal())
}

func DoUpdateArticle(id string, f ...interface{}) (*model.Article, error) {
	common.LockKey(id)
	defer common.UnlockKey(id)

	a, err := GetArticle(id)
	if err != nil {
		return nil, err
	}
	a.Extras = common.DefaultMap(a.Extras)
	if err := callOrSet(reflect.ValueOf(a), f...); err != nil {
		return nil, err
	}
	if a.Replies < 0 {
		a.Replies = 0
	}
	if a.Likes < 0 {
		a.Likes = 0
	}
	return a, m.db.Set(a.ID, a.Marshal())
}

func DoInsertArticle(rootID string, isReply bool, a model.Article) (A, R model.Article, E error) {
	if a.CreateTime.IsZero() || a.CreateTime == (time.Time{}) {
		a.CreateTime = time.Now()
	}
	if a.ID == "" {
		a.ID = ik.NewGeneralID().String()
	}

	common.LockKey(rootID)
	defer common.UnlockKey(rootID)

	root, err := GetArticle(rootID)
	if err != nil && err != model.ErrNotExisted {
		return model.Article{}, model.Article{}, err
	}

	if err == model.ErrNotExisted {
		if isReply {
			return model.Article{}, model.Article{}, err
		}
		root = &model.Article{
			ID:         rootID,
			EOC:        a.ID,
			ReplyEOC:   a.ID,
			CreateTime: time.Now(),
			Extras:     map[string]string{},
		}
	}

	root.Replies++
	root.Extras = common.DefaultMap(root.Extras)

	if ik.ParseID(rootID).Header() == ik.IDAuthor {
		// Inserting into user's timeline, which has some special cases

		// 1. This article will be the stick-on-top one
		if a.T_StickOnTop {
			root.Extras["stick_on_top"] = a.ID
		}

		// 2. This article is the first article of month X, so insert a checkpoint between X and X-1
		if x, y := ik.ParseID(rootID), ik.ParseID(root.NextID); y.Valid() {
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
	}

	if isReply {
		// The article is a reply to another feed, so insert it into root's reply chain
		if root.Asc == 1 {
			// Order asc
			switch root.ReplyEOC {
			case a.ID:
				// Already the last (newest) article
			case "":
				root.ReplyEOC, root.ReplyChain = a.ID, a.ID
			default:
				if common.LockAnotherKey(root.ReplyEOC, root.ID) {
					defer common.UnlockKey(root.ReplyEOC)
				}
				lastReply, err := GetArticle(root.ReplyEOC)
				if err != nil {
					E = err
					return
				}
				lastReply.NextReplyID = a.ID
				if err := m.db.Set(lastReply.ID, lastReply.Marshal()); err != nil {
					return model.Article{}, model.Article{}, err
				}
				root.ReplyEOC = a.ID
			}
		} else {
			// Order desc
			a.NextReplyID, root.ReplyChain = root.ReplyChain, a.ID
		}
	} else {
		// The article is a normal feed, so insert it into root's main chain/media chain
		a.NextID, root.NextID = root.NextID, a.ID
		if a.Media != "" {
			a.NextMediaID, root.NextMediaID = root.NextMediaID, a.ID
		}
	}

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return model.Article{}, model.Article{}, err
	}

	if err := m.db.Set(root.ID, root.Marshal()); err != nil {
		return model.Article{}, model.Article{}, err
	}

	return a, *root, nil
}

func DoSetFollowingSlot(rootID, followID string, toID, state string) error {
	common.LockKey(rootID)
	defer common.UnlockKey(rootID)

	a := model.Article{
		ID:         followID,
		Cmd:        model.CmdFollow,
		Extras:     map[string]string{toID: state},
		CreateTime: time.Now(),
	}

	root, err := GetArticle(rootID)
	if err != nil && err != model.ErrNotExisted {
		return err
	}

	if err == model.ErrNotExisted {
		root = &model.Article{
			ID:         rootID,
			EOC:        a.ID,
			CreateTime: time.Now(),
			Extras:     map[string]string{},
		}
	}

	root.Replies++
	root.Extras = common.DefaultMap(root.Extras)

	// The article contains following info, it won't go into any chains
	// instead root's Extras will record the index.
	// 'index' is the last element of the article's ArticleID: u/<user_id>/follow/<index>
	root.Extras[lastElemInCompID(a.ID)] = "1"

	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return err
	}

	return m.db.Set(root.ID, root.Marshal())
}

func DoUpdateOrInsertCmdArticle(rootID, id string, cmd, cmdValue, toSubject string) (updated, inserted bool, err error) {
	common.LockKey(id)
	defer common.UnlockKey(id)

	a, err := GetArticle(id)
	if err != nil {
		if err == model.ErrNotExisted {
			a := &model.Article{
				ID:         id,
				Cmd:        model.Cmd(cmd),
				Extras:     map[string]string{"to": toSubject, cmd: cmdValue},
				CreateTime: time.Now(),
			}

			if cmd == model.CmdLike {
				if toa, _ := GetArticle(toSubject); toa != nil {
					a.Media = toa.Media
				}
			}

			go DoInsertArticle(rootID, false, *a)
			return true, true, nil
		}
		return updated, inserted, err
	}

	updated = a.Extras[cmd] != cmdValue
	a.Extras[cmd] = cmdValue
	return updated, false, m.db.Set(a.ID, a.Marshal())
}
