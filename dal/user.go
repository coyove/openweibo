package dal

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

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func GetUser(id string) (*model.User, error) {
	if id == "" {
		return nil, fmt.Errorf("empty user id")
	}
	if u := m.weakUsers.Get(id); u != nil {
		return (*model.User)(u), nil
	}

	p, err := m.db.Get("u/" + id)
	if err != nil {
		return nil, err
	}

	if len(p) == 0 {
		return nil, model.ErrNotExisted
	}

	u, err := model.UnmarshalUser(p)
	if u != nil {
		u2 := *u
		m.weakUsers.Add(u.ID, unsafe.Pointer(&u2))
	}

	return u, err
}

func GetUserWithSettings(id string) (*model.User, error) {
	u, err := GetUser(id)
	if err != nil {
		return u, err
	}
	p, _ := m.db.Get("u/" + id + "/settings")
	u.SetSettings(model.UnmarshalUserSettings(p))
	return u, nil
}

func GetUserByContext(g *gin.Context) *model.User {
	u, _ := GetUserByToken(g.PostForm("api2_uid"))
	if u != nil && u.Banned {
		return nil
	}
	return u
}

func GetUserByToken(tok string) (*model.User, error) {
	if tok == "" {
		return nil, fmt.Errorf("invalid token")
	}

	x, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, err
	}

	for i := len(x) - 16; i >= 0; i -= 8 {
		common.Cfg.Blk.Decrypt(x[i:], x[i:])
	}

	parts := bytes.SplitN(x, []byte("\x00"), 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid token format")
	}

	session, id := parts[0], parts[1]
	u, err := GetUserWithSettings(string(id))
	if err != nil {
		return nil, err
	}

	if u.Session != string(session) {
		return nil, fmt.Errorf("invalid token session")
	}
	return u, nil
}

func ClearInbox(uid string) error {
	return Do(NewRequest(DoUpdateArticle,
		"ID", ik.NewID(ik.IDInbox, uid).String(),
		"ClearNextID", true,
	))
}

func MentionUserAndTags(a *model.Article, ids []string, tags []string) error {
	for _, id := range ids {
		if IsBlocking(id, a.Author) {
			return fmt.Errorf("author blocked")
		}

		if err := Do(NewRequest(DoInsertArticle,
			"ID", ik.NewID(ik.IDInbox, id).String(),
			"Article", model.Article{
				ID:  ik.NewGeneralID().String(),
				Cmd: model.CmdMention,
				Extras: map[string]string{
					"from":       a.Author,
					"article_id": a.ID,
				},
				CreateTime: time.Now(),
			},
		)); err != nil {
			return err
		}
		if err := Do(NewRequest(DoUpdateUser, "ID", id, "IncUnread", true)); err != nil {
			return err
		}
	}

	for _, tag := range tags {
		if err := Do(NewRequest(DoInsertArticle,
			"ID", ik.NewID(ik.IDTag, tag).String(),
			"Article", model.Article{
				ID:         ik.NewGeneralID().String(),
				ReferID:    a.ID,
				Media:      a.Media,
				CreateTime: time.Now(),
			},
		)); err != nil {
			return err
		}
		common.AddTagToSearch(tag)
	}
	return nil
}

func FollowUser(from, to string, following bool) (E error) {
	followID := makeFollowID(from, to)
	if following && IsBlocking(to, from) {
		// "from" wants "to" follow "to" but "to" blocked "from"
		return fmt.Errorf("follow/to-blocked")
	}

	updated := false
	defer func() {
		if E != nil || !updated {
			return
		}

		go func() {
			Do(NewRequest(DoUpdateUser, "ID", from, "IncDecFollowings", following))
			if !strings.HasPrefix(to, "#") {
				fromFollowToNotifyTo(from, to, following)
			}
		}()
	}()

	r := NewRequest(DoUpdateArticle,
		"ID", followID,
		"SetExtraKey", to,
		"SetExtraValue", strconv.FormatBool(following)+","+strconv.FormatInt(time.Now().Unix(), 10),
	)
	if err := Do(r); err != nil {
		if err == model.ErrNotExisted {
			updated = true
			if err := Do(NewRequest(DoInsertArticle,
				"ID", ik.NewID(ik.IDFollowing, from).String(),
				"Article", model.Article{
					ID:         followID,
					Cmd:        model.CmdFollow,
					Extras:     map[string]string{to: *r.UpdateArticleRequest.SetExtraValue},
					CreateTime: time.Now(),
				},
				"AsFollowing", true,
			)); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	Do(NewRequest(DoUpdateArticle,
		"ID", ik.NewID(ik.IDFollowing, from).String(),
		"SetExtraKey", lastElemInCompID(followID),
		"SetExtraValue", "1",
	))
	if !strings.HasPrefix(r.UpdateArticleRequest.Response.OldExtraValue, strconv.FormatBool(following)) {
		updated = true
	}
	return nil
}

func fromFollowToNotifyTo(from, to string, following bool) (E error) {
	if err := Do(NewRequest(DoUpdateUser, "ID", to, "IncDecFollowers", following)); err != nil {
		return err
	}
	return Do(NewRequest(DoUpdateOrInsertArticle,
		"ID", makeFollowedID(to, from),
		"ID2", from,
		"ChainID", ik.NewID(ik.IDFollower, to).String(),
		"Cmd", model.CmdFollowed,
		"Value", following))
}

func BlockUser(from, to string, blocking bool) (E error) {
	if blocking {
		if err := FollowUser(to, from, false); err != nil {
			log.Println("Block user:", to, "unfollow error:", err)
		}
	}

	return Do(NewRequest(DoUpdateOrInsertArticle,
		"ID", makeBlockID(from, to),
		"ID2", to,
		"ChainID", ik.NewID(ik.IDBlacklist, from).String(),
		"Cmd", model.CmdBlock,
		"Value", blocking))
}

func LikeArticle(from, to string, liking bool) (E error) {
	req := NewRequest(DoUpdateOrInsertArticle,
		"ID", makeLikeID(from, to),
		"ID2", to,
		"ChainID", ik.NewID(ik.IDLike, from).String(),
		"Cmd", model.CmdLike,
		"Value", liking,
	)
	if err := Do(req); err != nil {
		return err
	}
	if req.UpdateOrInsertArticleRequest.Response.Updated {
		go func() {
			r := NewRequest(DoUpdateArticle, "ID", to, "IncDecLikes", liking)
			if err := Do(r); err == nil {
				// if the author followed 'from', notify the author that his articles has been liked by 'from'
				if a := r.UpdateArticleRequest.Response.ArticleAuthor; IsFollowing(a, from) && liking {
					Do(NewRequest(DoInsertArticle,
						"ID", ik.NewID(ik.IDInbox, a).String(),
						"Article", model.Article{
							ID:  ik.NewGeneralID().String(),
							Cmd: model.CmdILike,
							Extras: map[string]string{
								"from":       from,
								"article_id": to,
							},
							CreateTime: time.Now(),
						}))
					Do(NewRequest(DoUpdateUser, "ID", a, "IncUnread", true))
				}
			}

		}()
	}
	return nil
}

type FollowingState struct {
	ID          string
	FullUser    *model.User
	Time        time.Time
	Followed    bool
	RevFollowed bool
	Liked       bool
	Blocked     bool
}

func GetRelationList(chain ik.ID, cursor string, n int) ([]FollowingState, string) {
	if cursor == "" {
		a, err := GetArticle(chain.String())
		if err != nil {
			if err != model.ErrNotExisted {
				log.Println("[GetRelationList] Failed to get chain [", chain, "]")
			}
			return nil, ""
		}
		cursor = a.NextID
	}

	res := []FollowingState{}
	start := time.Now()

	for len(res) < n && strings.HasPrefix(cursor, "u/") {
		if time.Since(start).Seconds() > 0.2 {
			log.Println("[GetRelationList] Break out slow walk [", cursor, "]")
			break
		}

		a, err := GetArticle(cursor)
		if err != nil {
			log.Println("[GetRelationList]", cursor, err)
			break
		}

		s := FollowingState{
			ID:          a.Extras["to"],
			Time:        a.CreateTime,
			Blocked:     a.Extras["block"] == "true",
			RevFollowed: a.Extras["followed"] == "true",
			Liked:       a.Extras["like"] == "true",
		}
		s.FullUser, _ = GetUser(s.ID)

		if chain.Header() == ik.IDFollower && s.RevFollowed {
			s.Followed = IsFollowing(chain.Tag(), s.ID)
		}

		res = append(res, s)
		cursor = a.NextID
	}

	sort.Slice(res, func(i, j int) bool { return res[i].Time.After(res[j].Time) })

	return res, cursor
}

func GetFollowingList(chain ik.ID, cursor string, n int, fulluser bool) ([]FollowingState, string) {
	var idx int
	var flags map[string]string

	if parts := strings.Split(cursor, "~"); len(parts) != 2 {
		// Start from the root article
		master, err := GetArticle(chain.String())
		if err != nil {
			if err != model.ErrNotExisted {
				log.Println("[GetRelationList] Failed to get chain [", chain, "]")
			}
			return nil, ""
		}
		flags = master.Extras
	} else {
		// Parse the cursor and 0 - 255 flags
		idx, _ = strconv.Atoi(parts[0])
		flags = common.Unpack256(parts[1])
		if idx > 255 || idx < 0 || flags == nil {
			return nil, ""
		}
	}

	res := []FollowingState{}
	start := time.Now()

	for who := chain.Tag(); len(res) < n && idx < 256; idx++ {
		if flags[strconv.Itoa(idx)] != "1" {
			continue
		}

		if time.Since(start).Seconds() > 0.2 {
			log.Println("[GetFollowingList] Break out slow walk [", cursor, "]")
			break
		}

		a, err := GetArticle("u/" + who + "/follow/" + strconv.Itoa(idx))
		if err != nil {
			log.Println("[GetFollowingList]", cursor, err)
			break
		}

		for k, v := range a.Extras {
			p := strings.Split(v, ",")
			if len(p) != 2 {
				continue
			}
			s := FollowingState{
				ID:       k,
				Time:     time.Unix(atoi64(p[1]), 0),
				Followed: atob(p[0]),
				Blocked:  IsBlocking(who, k),
			}
			if fulluser {
				if !strings.HasPrefix(k, "#") {
					s.FullUser, _ = GetUser(k)
				} else {
					s.FullUser = &model.User{
						ID: k,
					}
				}
			}
			res = append(res, s)
		}
	}

	sort.Slice(res, func(i, j int) bool { return res[i].Time.After(res[j].Time) })

	if idx > 255 {
		cursor = ""
	} else {
		cursor = strconv.Itoa(idx) + "~" + common.Pack256(flags)
	}

	return res, cursor
}

func IsFollowing(from, to string) bool {
	p, _ := GetArticle(makeFollowID(from, to))
	return p != nil && strings.HasPrefix(p.Extras[to], "true")
}

func IsBlocking(from, to string) bool {
	p, _ := GetArticle(makeBlockID(from, to))
	return p != nil && p.Extras["block"] == "true"
}

func IsLiking(from, to string) bool {
	p, _ := GetArticle(makeLikeID(from, to))
	return p != nil && p.Extras["like"] == "true"
}
