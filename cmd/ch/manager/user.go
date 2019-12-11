package manager

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/mv"
)

func (m *Manager) GetUser(id string) (*mv.User, error) {
	p, err := m.db.Get("u/" + id)
	if err != nil {
		return nil, err
	}
	return mv.UnmarshalUser(p)
}

func (m *Manager) GetUserByToken(tok string) (*mv.User, error) {
	if tok == "" {
		return nil, fmt.Errorf("invalid token")
	}

	x, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, err
	}

	for i := len(x) - 16; i >= 0; i -= 8 {
		config.Cfg.Blk.Decrypt(x[i:], x[i:])
	}

	parts := bytes.SplitN(x, []byte("\x00"), 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	session, id := parts[0], parts[1]
	u, err := m.GetUser(string(id))
	if err != nil {
		return nil, err
	}

	if u.Session != string(session) {
		return nil, fmt.Errorf("invalid token session")
	}
	return u, nil
}

func (m *Manager) IsBanned(id string) bool {
	u, err := m.GetUser(id)
	if err != nil {
		return true
	}
	return u.Banned
}

func (m *Manager) SetUser(u *mv.User) error {
	if u.ID == "" {
		return nil
	}
	return m.db.Set("u/"+u.ID, u.Marshal())
}

func (m *Manager) LockUserID(id string) {
	m.db.Lock(id)
}

func (m *Manager) UnlockUserID(id string) {
	m.db.Unlock(id)
}

func (m *Manager) UpdateUser(id string, cb func(u *mv.User) error) error {
	m.db.Lock(id)
	defer m.db.Unlock(id)
	return m.UpdateUser_unlock(id, cb)
}

func (m *Manager) UpdateUser_unlock(id string, cb func(u *mv.User) error) error {
	u, err := m.GetUser(id)
	if err != nil {
		return err
	}
	if err := cb(u); err != nil {
		return err
	}
	return m.SetUser(u)
}

func (m *Manager) MentionUser(a *mv.Article, id string) error {
	if err := m.insertArticle(ident.NewID(ident.IDTagInbox).SetTag(id).String(), &mv.Article{
		ID:  ident.NewGeneralID().String(),
		Cmd: mv.CmdMention,
		Extras: map[string]string{
			"from":       a.Author,
			"article_id": a.ID,
		},
		CreateTime: time.Now(),
	}, false); err != nil {
		return err
	}
	return m.UpdateUser(id, func(u *mv.User) error {
		u.Unread++
		return nil
	})
}

func (m *Manager) FollowUser_unlock(from, to string, following bool) error {
	u, err := m.GetUser(from)
	if err != nil {
		return err
	}

	root := u.FollowingChain
	if u.FollowingChain == "" {
		a := &mv.Article{
			ID:         ident.NewGeneralID().String(),
			Cmd:        mv.CmdFollow,
			CreateTime: time.Now(),
		}
		if err := m.db.Set(a.ID, a.Marshal()); err != nil {
			return err
		}
		if err := m.UpdateUser_unlock(from, func(u *mv.User) error {
			u.FollowingChain = a.ID
			return nil
		}); err != nil {
			return err
		}
		root = a.ID
	}

	followID := makeFollowID(from, to)

	defer func() {

		go m.UpdateUser(to, func(u *mv.User) error {
			if following {
				u.Followers++
			} else {
				dec0(&u.Followers)
			}
			return nil
		})

		go m.UpdateUser(from, func(u *mv.User) error {
			if following {
				u.Followings++
			} else {
				dec0(&u.Followings)
			}
			return nil
		})
	}()

	if a, _ := m.GetArticle(followID); a != nil {
		state := strconv.FormatBool(following)
		if a.Extras["follow"] == state {
			return nil
		}
		a.Extras["follow"] = state
		return m.db.Set(a.ID, a.Marshal())
	}

	if err := m.insertArticle(root, &mv.Article{
		ID:  followID,
		Cmd: mv.CmdFollow,
		Extras: map[string]string{
			"to":     to,
			"follow": strconv.FormatBool(following),
		},
		CreateTime: time.Now(),
	}, false); err != nil {
		return err
	}

	return nil
}

type FollowingState struct {
	ID       string
	Time     time.Time
	Followed bool
}

func (m *Manager) GetFollowingList(u *mv.User, cursor string, n int) ([]FollowingState, string) {
	if u.FollowingChain == "" {
		if cursor != "" {
			if u, _ := m.GetUser(cursor[strings.LastIndex(cursor, "/")+1:]); u != nil {
				return []FollowingState{{
					ID:   u.ID,
					Time: u.Signup,
				}}, ""
			}
		}
		return nil, ""
	}

	if cursor == "" {
		a, err := m.GetArticle(u.FollowingChain)
		if err != nil {
			log.Println("[GetFollowingList] Failed to get chain", u, "[", u.FollowingChain, "]")
			return nil, ""
		}
		cursor = a.NextID
	}

	res := []FollowingState{}
	start := time.Now()
	startCursor := cursor

	for len(res) < n && cursor != "" {
		if time.Since(start).Seconds() > 0.2 {
			log.Println("[GetFollowingList] Break out slow walk", u, "[", cursor, "]")
			break
		}
		a, err := m.GetArticle(cursor)
		if err != nil {
			if cursor == startCursor {
				to := cursor[strings.LastIndex(cursor, "/")+1:]
				u, err := m.GetUser(to)
				if err == nil {
					res = append(res, FollowingState{
						ID:   to,
						Time: u.Signup,
					})
				}
			}
			log.Println("[GetFollowingList]", err)
			break
		}
		if a.Extras["follow"] == "true" {
			res = append(res, FollowingState{
				ID:       a.Extras["to"],
				Time:     a.CreateTime,
				Followed: true,
			})
		} else if cursor == startCursor {
			res = append(res, FollowingState{
				ID:       a.Extras["to"],
				Time:     a.CreateTime,
				Followed: false,
			})
		}

		cursor = a.NextID
	}

	return res, cursor
}

func (m *Manager) IsFollowing(from, to string) bool {
	p, _ := m.db.Get(makeFollowID(from, to))
	return bytes.Equal(p, []byte("true"))
}

// func (m *Manager) UpvoteArticle(uid, aid string, upvote bool) error {
// 	relationID := makeUserArticleRelationID(uid, aid)
//
// 	if a, _ := m.GetArticle(relationID); a != nil {
// 		a.Extras["upvote"] = strconv.FormatBool(upvote)
// 		a.Extras["forward"] = strconv.FormatBool(forward)
// 		return m.db.Set(a.ID, a.Marshal())
// 	}
//
// 	if err := m.db.Set(relationID, (&mv.Article{
// 		ID:  relationID,
// 		Cmd: mv.CmdUARelation,
// 		Extras: map[string]string{
// 			"article_id": aid,
// 			"upvote":     strconv.FormatBool(upvote),
// 			"forward":    strconv.FormatBool(forward),
// 		},
// 		CreateTime: time.Now(),
// 	}).Marshal()); err != nil {
// 		return err
// 	}
//
// 	return m.UpdateArticle(aid, func(a *mv.Article) error {
// 		if upvote {
// 		}
// 		return nil
// 	})
// }
