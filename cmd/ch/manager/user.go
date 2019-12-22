package manager

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/coyove/iis/cmd/ch/config"
	"github.com/coyove/iis/cmd/ch/ident"
	"github.com/coyove/iis/cmd/ch/mv"
)

func (m *Manager) GetUser(id string) (*mv.User, error) {
	if id == "" {
		return nil, mv.ErrNotExisted
	}
	if u := m.weakUsers.Get(id); u != nil {
		return (*mv.User)(u), nil
	}

	p, err := m.db.Get("u/" + id)
	if err != nil {
		return nil, err
	}
	u, err := mv.UnmarshalUser(p)
	if u != nil {
		u2 := *u
		m.weakUsers.Add(u.ID, unsafe.Pointer(&u2))
	}
	return u, err
}

func (m *Manager) SetUser(u *mv.User) error {
	if u.ID == "" {
		return nil
	}
	m.weakUsers.Delete(u.ID)
	return m.db.Set("u/"+u.ID, u.Marshal())
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
	if len(parts) < 2 {
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

func (m *Manager) Lock(id string) {
	m.db.Lock(id)
}

func (m *Manager) Unlock(id string) {
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

func (m *Manager) MentionUserAndTags(a *mv.Article, ids []string, tags []string) error {
	for _, id := range ids {
		if m.IsBlocking(id, a.Author) {
			return fmt.Errorf("author blocked")
		}

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
		if err := m.UpdateUser(id, func(u *mv.User) error {
			u.Unread++
			return nil
		}); err != nil {
			return err
		}
	}

	for _, tag := range tags {
		if err := m.insertArticle(ident.NewID(ident.IDTagTag).SetTag(tag).String(), &mv.Article{
			ID:         ident.NewGeneralID().String(),
			ReferID:    a.ID,
			CreateTime: time.Now(),
		}, false); err != nil {
			return err
		}
		mv.AddTagToSearch(tag)
	}
	return nil
}

func (m *Manager) createUserChain(u *mv.User, cmd mv.Cmd) error {
	a := &mv.Article{
		ID:         ident.NewGeneralID().String(),
		Cmd:        cmd,
		CreateTime: time.Now(),
	}
	if err := m.db.Set(a.ID, a.Marshal()); err != nil {
		return err
	}
	return m.UpdateUser_unlock(u.ID, func(uu *mv.User) error {
		switch cmd {
		case mv.CmdFollow:
			u.FollowingChain, uu.FollowingChain = a.ID, a.ID
		case mv.CmdBlock:
			u.BlockingChain, uu.BlockingChain = a.ID, a.ID
		case mv.CmdFollowed:
			u.FollowerChain, uu.FollowerChain = a.ID, a.ID
		default:
			panic(cmd)
		}
		return nil
	})
}

func (m *Manager) FollowUser_unlock(from, to string, following bool) (E error) {
	u, err := m.GetUser(from)
	if err != nil {
		return err
	}

	if u.FollowingChain == "" {
		if err := m.createUserChain(u, mv.CmdFollow); err != nil {
			return err
		}
	}
	root := u.FollowingChain

	followID := MakeFollowID(from, to)
	m.db.Lock(followID)
	defer m.db.Unlock(followID)

	if following && m.IsBlocking(to, from) {
		// "from" wants "to" follow "to" but "to" blocked "from"
		return fmt.Errorf("follow/to-blocked")
	}

	defer func() {
		if E != nil {
			return
		}

		go func() {
			m.UpdateUser(from, func(u *mv.User) error {
				if following {
					u.Followings++
				} else {
					dec0(&u.Followings)
				}
				return nil
			})
			if !strings.HasPrefix(to, "#") {
				m.fromFollowToNotifyTo(from, to, following)
			}
		}()
	}()

	state := strconv.FormatBool(following) + "," + strconv.FormatInt(time.Now().Unix(), 10)
	if a, _ := m.GetArticle(followID); a != nil {
		a.Extras[to] = state
		return m.db.Set(a.ID, a.Marshal())
	}

	if err := m.insertArticle(root, &mv.Article{
		ID:         followID,
		Cmd:        mv.CmdFollow,
		Extras:     map[string]string{to: state},
		CreateTime: time.Now(),
	}, false); err != nil {
		return err
	}

	return nil
}

func (m *Manager) fromFollowToNotifyTo(from, to string, following bool) (E error) {
	m.db.Lock(to)
	defer m.db.Unlock(to)

	u, err := m.GetUser(to)
	if err != nil {
		return err
	}

	if following {
		u.Followers++
	} else {
		dec0(&u.Followers)
	}

	if err := m.SetUser(u); err != nil {
		return err
	}

	if u.FollowerChain == "" {
		if err := m.createUserChain(u, mv.CmdFollowed); err != nil {
			return err
		}
	}

	followID := makeFollowedID(to, from)
	m.db.Lock(followID)
	defer m.db.Unlock(followID)

	if a, _ := m.GetArticle(followID); a != nil {
		a.Extras["followed"] = strconv.FormatBool(following)
		return m.db.Set(a.ID, a.Marshal())
	}

	if err := m.insertArticle(u.FollowerChain, &mv.Article{
		ID:  followID,
		Cmd: mv.CmdFollowed,
		Extras: map[string]string{
			"to":       from,
			"followed": strconv.FormatBool(following),
		},
		CreateTime: time.Now(),
	}, false); err != nil {
		return err
	}
	return nil
}

func (m *Manager) BlockUser_unlock(from, to string, blocking bool) (E error) {
	u, err := m.GetUser(from)
	if err != nil {
		return err
	}

	if u.BlockingChain == "" {
		if err := m.createUserChain(u, mv.CmdBlock); err != nil {
			return err
		}
	}
	root := u.BlockingChain

	if blocking {
		if err := m.FollowUser_unlock(to, from, false); err != nil {
			log.Println("Block user:", to, "unfollow error:", err)
		}
	}

	followID := makeBlockID(from, to)
	m.db.Lock(followID)
	defer m.db.Unlock(followID)

	if a, _ := m.GetArticle(followID); a != nil {
		state := strconv.FormatBool(blocking)
		if a.Extras["block"] == state {
			return nil
		}
		a.Extras["block"] = state
		return m.db.Set(a.ID, a.Marshal())
	}

	if err := m.insertArticle(root, &mv.Article{
		ID:  followID,
		Cmd: mv.CmdBlock,
		Extras: map[string]string{
			"to":    to,
			"block": strconv.FormatBool(blocking),
		},
		CreateTime: time.Now(),
	}, false); err != nil {
		return err
	}

	return nil
}

type FollowingState struct {
	ID          string
	Time        time.Time
	Followed    bool
	RevFollowed bool
	Blocked     bool
}

func (m *Manager) GetFollowingList(chain string, cursor string, n int) ([]FollowingState, string) {
	if chain == "" {
		return nil, ""
	}

	if cursor == "" {
		a, err := m.GetArticle(chain)
		if err != nil {
			log.Println("[GetFollowingList] Failed to get chain [", chain, "]")
			return nil, ""
		}
		cursor = a.NextID
	}

	res := []FollowingState{}
	start := time.Now()

	for len(res) < n && cursor != "" {
		if time.Since(start).Seconds() > 0.2 {
			log.Println("[GetFollowingList] Break out slow walk [", cursor, "]")
			break
		}

		a, err := m.GetArticle(cursor)
		if err != nil {
			log.Println("[GetFollowingList]", cursor, err)
			break
		}

		if a.Cmd == mv.CmdFollow {
			for k, v := range a.Extras {
				p := strings.Split(v, ",")
				if len(p) != 2 {
					continue
				}
				res = append(res, FollowingState{
					ID:       k,
					Time:     time.Unix(atoi64(p[1]), 0),
					Followed: atob(p[0]),
				})
			}
		} else {
			res = append(res, FollowingState{
				ID:          a.Extras["to"],
				Time:        a.CreateTime,
				Blocked:     a.Extras["block"] == "true",
				RevFollowed: a.Extras["followed"] == "true",
			})
		}

		cursor = a.NextID
	}

	sort.Slice(res, func(i, j int) bool { return res[i].Time.After(res[j].Time) })

	return res, cursor
}

func (m *Manager) IsFollowing(from, to string) bool {
	p, _ := m.GetArticle(MakeFollowID(from, to))
	return p != nil && strings.HasPrefix(p.Extras[to], "true")
}

func (m *Manager) IsBlocking(from, to string) bool {
	p, _ := m.GetArticle(makeBlockID(from, to))
	return p != nil && p.Extras["block"] == "true"
}
