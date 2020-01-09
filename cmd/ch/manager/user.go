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
	"github.com/gin-gonic/gin"
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
		if u.FollowingChain != "" {
			go func() {
				fc, _ := m.GetArticle(u.FollowingChain)
				id := ident.NewID(ident.IDTagFollowChain).SetTag(u.ID).String()
				if fc != nil {
					for !strings.HasPrefix(fc.NextID, "u/") {
						fc2, _ := m.GetArticle(fc.NextID)
						if fc2 != nil {
							fc = fc2
						} else {
							log.Println(u.ID, "following chain broken")
							break
						}
					}
					a := &mv.Article{
						ID:     id,
						NextID: fc.NextID,
					}
					if _, err := m.GetArticle(a.ID); err == mv.ErrNotExisted {
						m.db.Set(a.ID, a.Marshal())
					}
				}
			}()
		}
	}

	return u, err
}

func (m *Manager) GetUserWithSettings(id string) (*mv.User, error) {
	u, err := m.GetUser(id)
	if err != nil {
		return u, err
	}
	p, _ := m.db.Get("u/" + id + "/settings")
	u.SetSettings(mv.UnmarshalUserSettings(p))
	return u, nil
}

func (m *Manager) SetUser(u *mv.User) error {
	if u.ID == "" {
		return nil
	}
	m.weakUsers.Delete(u.ID)
	return m.db.Set("u/"+u.ID, u.Marshal())
}

func (m *Manager) GetUserByContext(g *gin.Context) *mv.User {
	u, _ := m.GetUserByToken(g.PostForm("api2_uid"))
	if u != nil && u.Banned {
		return nil
	}
	return u
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
	u, err := m.GetUserWithSettings(string(id))
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

func (m *Manager) UpdateUserSettings(id string, cb func(u *mv.UserSettings) error) error {
	m.db.Lock(id)
	defer m.db.Unlock(id)
	sid := "u/" + id + "/settings"
	p, _ := m.db.Get(sid)
	s := mv.UnmarshalUserSettings(p)
	if err := cb(&s); err != nil {
		return err
	}
	return m.db.Set(sid, s.Marshal())
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
			Media:      a.Media,
			CreateTime: time.Now(),
		}, false); err != nil {
			return err
		}
		mv.AddTagToSearch(tag)
	}
	return nil
}

func (m *Manager) FollowUser_unlock(from, to string, following bool) (E error) {
	followID := MakeFollowID(from, to)
	if following && m.IsBlocking(to, from) {
		// "from" wants "to" follow "to" but "to" blocked "from"
		return fmt.Errorf("follow/to-blocked")
	}

	m.db.Lock(followID)
	defer m.db.Unlock(followID)

	updated := false
	defer func() {
		if E != nil || !updated {
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
		if !strings.HasPrefix(a.Extras[to], strconv.FormatBool(following)) {
			a.Extras[to] = state
			updated = true
		}
		return m.db.Set(a.ID, a.Marshal())
	}

	updated = true
	if err := m.insertArticle(
		ident.NewID(ident.IDTagFollowChain).SetTag(from).String(),
		&mv.Article{
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
	if err := m.UpdateUser(to, func(u *mv.User) error {
		if following {
			u.Followers++
		} else {
			dec0(&u.Followers)
		}
		return nil
	}); err != nil {
		return err
	}
	_, err := m.insertChainOrUpdate(
		makeFollowedID(to, from),
		ident.NewID(ident.IDTagFollowerChain).SetTag(to).String(),
		from,
		mv.CmdFollowed,
		following)
	return err
}

func (m *Manager) BlockUser_unlock(from, to string, blocking bool) (E error) {
	if blocking {
		if err := m.FollowUser_unlock(to, from, false); err != nil {
			log.Println("Block user:", to, "unfollow error:", err)
		}
	}

	_, err := m.insertChainOrUpdate(
		makeBlockID(from, to),
		ident.NewID(ident.IDTagBlockChain).SetTag(from).String(),
		to,
		mv.CmdBlock,
		blocking)
	return err
}

func (m *Manager) LikeArticle_unlock(from, to string, liking bool) (E error) {
	updated, err := m.insertChainOrUpdate(
		makeLikeID(from, to),
		ident.NewID(ident.IDTagLikeChain).SetTag(from).String(),
		to,
		mv.CmdLike,
		liking)
	if err != nil {
		return err
	}
	if updated {
		return m.UpdateArticle(to, func(a *mv.Article) error {
			if liking {
				a.Likes++
			} else {
				dec0(&a.Likes)
			}
			return nil
		})
	}
	return nil
}

func (m *Manager) insertChainOrUpdate(aid, chainid string, to string, cmd mv.Cmd, value bool) (updated bool, E error) {
	m.db.Lock(aid)
	defer m.db.Unlock(aid)

	if a, _ := m.GetArticle(aid); a != nil {
		state := strconv.FormatBool(value)
		if a.Extras[string(cmd)] == state {
			return false, nil
		}
		if a.Extras == nil {
			a.Extras = map[string]string{}
		}
		a.Extras[string(cmd)] = state
		return true, m.db.Set(a.ID, a.Marshal())
	}

	a := &mv.Article{
		ID:  aid,
		Cmd: cmd,
		Extras: map[string]string{
			"to":        to,
			string(cmd): strconv.FormatBool(value),
		},
		CreateTime: time.Now(),
	}

	if cmd == mv.CmdLike {
		toa, _ := m.GetArticle(to)
		if toa != nil {
			a.Media = toa.Media
		}
	}

	return true, m.insertArticle(chainid, a, false)
}

type FollowingState struct {
	ID          string
	Time        time.Time
	Followed    bool
	RevFollowed bool
	Liked       bool
	Blocked     bool
}

func (m *Manager) GetFollowingList(chain ident.ID, cursor string, n int) ([]FollowingState, string) {
	if cursor == "" {
		a, err := m.GetArticle(chain.String())
		if err != nil {
			if err != mv.ErrNotExisted {
				log.Println("[GetFollowingList] Failed to get chain [", chain, "]")
			}
			return nil, ""
		}
		cursor = a.NextID
	}

	res := []FollowingState{}
	start := time.Now()

	for len(res) < n && strings.HasPrefix(cursor, "u/") {
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
				Liked:       a.Extras["like"] == "true",
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

func (m *Manager) IsLiking(from, to string) bool {
	p, _ := m.GetArticle(makeLikeID(from, to))
	return p != nil && p.Extras["like"] == "true"
}
