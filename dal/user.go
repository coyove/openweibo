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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal/forgettable/goforget"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
	"github.com/gin-gonic/gin"
)

func init() {
	model.DalIsBlocking = IsBlocking
	model.DalIsFollowing = IsFollowing
	model.DalIsFollowingWithAcceptance = IsFollowingWithAcceptance
}

func GetUser(id string) (*model.User, error) {
	if id == "" {
		return nil, fmt.Errorf("empty user id")
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
	}
	return u, err
}

func GetUserWithSettings(id string) (*model.User, error) {
	u, err := GetUser(id)
	if err != nil {
		return u, err
	}
	u.SetSettings(GetUserSettings(id))
	return u, nil
}

func GetUserSettings(id string) model.UserSettings {
	p, _ := m.db.Get("u/" + id + "/settings")
	return model.UnmarshalUserSettings(p)
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
	_, err := DoUpdateArticle(&UpdateArticleRequest{
		ID:          ik.NewID(ik.IDInbox, uid).String(),
		ClearNextID: aws.Bool(true),
	})
	return err
}

func MentionUserAndTags(a *model.Article, ids []string, tags []string) error {
	for _, id := range ids {
		if IsBlocking(id, a.Author) {
			return fmt.Errorf("author blocked")
		}

		if GetUserSettings(id).OnlyMyFollowingsCanMention && !IsFollowing(id, a.Author) {
			continue
		}

		if _, err := DoInsertArticle(&InsertArticleRequest{
			ID: ik.NewID(ik.IDInbox, id).String(),
			Article: model.Article{
				ID:  ik.NewGeneralID().String(),
				Cmd: model.CmdInboxMention,
				Extras: map[string]string{
					"from":       a.Author,
					"article_id": a.ID,
				},
			},
		}); err != nil {
			return err
		}

		if _, err := DoUpdateUser(&UpdateUserRequest{ID: id, IncDecUnread: aws.Bool(true)}); err != nil {
			return err
		}
	}

	for _, tag := range tags {
		if _, err := DoInsertArticle(&InsertArticleRequest{
			ID: ik.NewID(ik.IDTag, tag).String(),
			Article: model.Article{
				ID:      ik.NewGeneralID().String(),
				ReferID: a.ID,
				Media:   a.Media,
			},
		}); err != nil {
			return err
		}
		common.AddTagToSearch(tag)
		goforget.Incr("tagheat", tag)
	}
	return nil
}

func FollowUser(from, to string, following bool) (E error) {
	followID := makeFollowID(from, to)
	if following {
		if IsBlocking(to, from) {
			// "from" wants to follow "to" but "to" blocked "from"
			return fmt.Errorf("follow/to-blocked")
		}
		if GetUserSettings(to).OnlyMyFollowingsCanFollow && !IsFollowing(to, from) {
			return fmt.Errorf("follow/to-following-required")
		}
	}

	updated := false
	defer func() {
		if E != nil || !updated {
			return
		}

		go func() {
			DoUpdateUser(&UpdateUserRequest{ID: from, IncDecFollowings: aws.Bool(following)})
			if !strings.HasPrefix(to, "#") {
				notifyNewFollower(from, to, following)
			}
		}()
	}()

	state := strconv.FormatBool(following) + "," + strconv.FormatInt(time.Now().Unix(), 10)
	oldValue, err := DoUpdateArticleExtra(&UpdateArticleExtraRequest{
		ID:            followID,
		SetExtraKey:   to,
		SetExtraValue: state,
	})
	if err != nil {
		if err != model.ErrNotExisted {
			return err
		}
		updated = true
		_, err := DoInsertArticle(&InsertArticleRequest{
			ID: ik.NewID(ik.IDFollowing, from).String(),
			Article: model.Article{
				ID:     followID,
				Cmd:    model.CmdFollow,
				Extras: map[string]string{to: state},
			},
			AsFollowingSlot: true,
		})
		return err
	}

	DoUpdateArticleExtra(&UpdateArticleExtraRequest{
		ID:            ik.NewID(ik.IDFollowing, from).String(),
		SetExtraKey:   lastElemInCompID(followID),
		SetExtraValue: "1",
	})

	if !strings.HasPrefix(oldValue, strconv.FormatBool(following)) {
		updated = true
	}
	return nil
}

func notifyNewFollower(from, to string, following bool) (E error) {
	updated, err := DoUpdateOrInsertCmdArticle(&UpdateOrInsertCmdArticleRequest{
		ArticleID:          makeFollowedID(to, from),
		ToSubject:          from,
		InsertUnderChainID: ik.NewID(ik.IDFollower, to).String(),
		Cmd:                model.CmdFollowed,
		CmdValue:           following,
	})
	if err != nil {
		return err
	}
	if updated {
		if _, err := DoUpdateUser(&UpdateUserRequest{ID: to, IncDecFollowers: aws.Bool(following)}); err != nil {
			return err
		}
	}
	return nil
}

func BlockUser(from, to string, blocking bool) (E error) {
	if blocking {
		if err := FollowUser(to, from, false); err != nil {
			log.Println("Block user:", to, "unfollow error:", err)
		}
		if err := AcceptUser(from, to, false); err != nil {
			log.Println("Unaccept user:", to, "unfollow error:", err)
		}
	}

	_, err := DoUpdateOrInsertCmdArticle(&UpdateOrInsertCmdArticleRequest{
		ArticleID:          makeBlockID(from, to),
		ToSubject:          to,
		InsertUnderChainID: ik.NewID(ik.IDBlacklist, from).String(),
		Cmd:                model.CmdBlock,
		CmdValue:           blocking,
	})
	return err
}

func AcceptUser(from, to string, accept bool) (E error) {
	id := makeFollowerAcceptanceID(from, to)
	if accept {
		go func() {
			DoInsertArticle(&InsertArticleRequest{
				ID: ik.NewID(ik.IDInbox, to).String(),
				Article: model.Article{
					ID:     ik.NewGeneralID().String(),
					Cmd:    model.CmdInboxFwAccepted,
					Extras: map[string]string{"from": from},
				},
			})
			DoUpdateUser(&UpdateUserRequest{ID: to, IncDecUnread: aws.Bool(true)})
		}()
	}
	return m.db.Set(id, (&model.Article{
		ID:     id,
		Extras: map[string]string{"accept": strconv.FormatBool(accept)},
	}).Marshal())
}

func LikeArticle(from, to string, liking bool) (E error) {
	updated, err := DoUpdateOrInsertCmdArticle(&UpdateOrInsertCmdArticleRequest{
		ArticleID:          makeLikeID(from, to),
		InsertUnderChainID: ik.NewID(ik.IDLike, from).String(),
		Cmd:                model.CmdLike,
		ToSubject:          to,
		CmdValue:           liking,
	})
	if err != nil {
		return err
	}
	if updated {
		go func() {
			a, err := DoUpdateArticle(&UpdateArticleRequest{ID: to, IncDecLikes: aws.Bool(liking)})
			if !(err == nil && IsFollowing(a.Author, from) && liking) {
				return
			}
			// if the author followed 'from', notify the author that his articles has been liked by 'from'
			DoInsertArticle(&InsertArticleRequest{
				ID: ik.NewID(ik.IDInbox, a.Author).String(),
				Article: model.Article{
					ID:  ik.NewGeneralID().String(),
					Cmd: model.CmdInboxLike,
					Extras: map[string]string{
						"from":       from,
						"article_id": to,
					},
				},
			})
			DoUpdateUser(&UpdateUserRequest{ID: a.Author, IncDecUnread: aws.Bool(true)})
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
	Accepted    bool
	// Relationship
	CommonFollowing  bool
	TwoHopsFollowing bool
}

func GetRelationList(u *model.User, chain ik.ID, cursor string, n int) ([]FollowingState, string) {
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
			log.Println("[GetRelationList] Break out slow walk [", chain.Tag(), "]")
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
			s.RevFollowed, s.Accepted = IsFollowingWithAcceptance(s.ID, u)
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
			log.Println("[GetFollowingList] Break out slow walk [", chain.Tag(), "]")
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
					s.FullUser = &model.User{ID: k}
				}
				if s.FullUser == nil {
					continue
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

func IsFollowingWithAcceptance(from string, to *model.User) (following bool, accepted bool) {
	if from == to.ID {
		return true, true
	}
	at := to.Settings().FollowerNeedsAcceptance
	if at == (time.Time{}) || at.IsZero() {
		return IsFollowing(from, to.ID), true // 'to' didn't have the switch on, so no acceptance needed for 'from'
	}
	p, _ := GetArticle(makeFollowID(from, to.ID))
	if p == nil {
		return false, false
	}
	if !strings.HasPrefix(p.Extras[to.ID], "true,") {
		return false, false // didn't follow 'to' at all
	}
	ts, err := strconv.ParseInt(p.Extras[to.ID][5:], 10, 64)
	if err != nil {
		return false, false
	}
	if time.Unix(ts, 0).Before(at) {
		return true, true // 'from' followed 'to' before 'to' turned on the switch, so 'from' gains acceptance automatically
	}
	accept, _ := GetArticle(makeFollowerAcceptanceID(to.ID, from))
	if accept == nil {
		return true, false
	}
	return true, accept.Extras["accept"] == "true"
}

func IsBlocking(from, to string) bool {
	p, _ := GetArticle(makeBlockID(from, to))
	return p != nil && p.Extras["block"] == "true"
}

func IsLiking(from, to string) bool {
	p, _ := GetArticle(makeLikeID(from, to))
	return p != nil && p.Extras["like"] == "true"
}

func LastActiveTime(uid string) time.Time {
	v, _ := m.activeUsers.Get("u/" + uid + "/last_active")
	t, _ := strconv.ParseInt(string(v), 10, 64)
	if t == 0 {
		return time.Time{}
	}
	return time.Unix(t, 0)
}

func MarkUserActive(uid string) {
	m.activeUsers.Add("u/"+uid+"/last_active", []byte(strconv.FormatInt(time.Now().Unix(), 10)))
}
