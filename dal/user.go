package dal

import (
	"fmt"
	"github.com/coyove/iis/common/bits"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal/tagrank"
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
	return getterUser(m.db.Get, id)
}

func WeakGetUser(id string) (*model.User, error) {
	return getterUser(m.db.WeakGet, id)
}

func getterUser(getter func(string) ([]byte, error), id string) (*model.User, error) {
	if id == "" {
		return nil, fmt.Errorf("empty user id")
	}

	p, err := getter("u/" + id)
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

func GetUserByContext(g *gin.Context) *model.User {
	u, _ := GetUserByToken(g.PostForm("api2_uid"), g.GetBool("allow-api"))
	if u != nil && u.Banned {
		return nil
	}
	return u
}

func GetUserByToken(tok string, allowAPI bool) (*model.User, error) {
	id, session, err := ik.ParseUserToken(tok)
	if err != nil {
		return nil, err
	}

	u, err := GetUser(string(id))
	if err != nil {
		return nil, err
	}

	if allowAPI && tok == u.APIToken {
		u.SetIsAPI(true)
		return u, nil
	}

	if u.Session != string(session) {
		return nil, fmt.Errorf("invalid token session")
	}
	return u, nil
}

func ClearInbox(uid string) error {
	_, err := DoUpdateArticle(ik.NewID(ik.IDInbox, uid).String(), func(a *model.Article) {
		a.NextID, a.NextMediaID = "", ""
	})
	return err
}

func MentionUserAndTags(a *model.Article, ids []string, tags []string) error {
	for _, id := range ids {
		if IsBlocking(id, a.Author) {
			return fmt.Errorf("author blocked")
		}

		if to, _ := GetUser(id); to != nil && to.NotifyFollowerActOnly == 1 && !IsFollowing(id, a.Author) {
			continue
		}

		if _, _, err := DoInsertArticle(ik.NewID(ik.IDInbox, id).String(), false, model.Article{
			Cmd:    model.CmdInboxMention,
			Extras: map[string]string{"from": a.Author, "article_id": a.ID},
		}); err != nil {
			return err
		}

		if err := IncUnread(id); err != nil {
			return err
		}
	}

	for _, tag := range tags {
		_, root, err := DoInsertArticle(ik.NewID(ik.IDTag, tag).String(), false, model.Article{
			ReferID: a.ID, Media: a.Media,
		})
		if err != nil {
			return err
		}
		model.IndexTag(tag)
		tagrank.Update(tag, root.CreateTime, root.Replies)
	}
	return nil
}

func FollowUser(from, to string, following bool) (E error) {
	followID := makeFollowID(from, to)
	updated := false
	defer func() {
		if E != nil || !updated {
			return
		}

		go func() {
			DoUpdateUser(from, func(u *model.User) { u.Followings += int32(common.BoolInt2(following)) })
			if !strings.HasPrefix(to, "#") {
				notifyNewFollower(from, to, following)

				if toUser, _ := WeakGetUser(to); toUser != nil && toUser.FollowApply != 0 && following {
					if _, err := WeakGetArticle(makeFollowerAcceptanceID(to, from)); err != model.ErrNotExisted {
						return
					}

					AcceptUser(to, from, false)
					DoInsertArticle(ik.NewID(ik.IDInbox, to).String(), false, model.Article{
						Cmd: model.CmdInboxFwApply, Extras: map[string]string{"from": from},
					})
					IncUnread(to)
				}
			}
		}()
	}()

	state := strconv.FormatBool(following) + "," + strconv.FormatInt(time.Now().Unix(), 10)
	var oldValue string
	if _, err := DoUpdateArticle(followID, func(a *model.Article) { oldValue, a.Extras[to] = a.Extras[to], state }); err != nil {
		if err != model.ErrNotExisted {
			return err
		}
		updated = true
		return DoSetFollowingSlot(ik.NewID(ik.IDFollowing, from).String(), followID, to, state)
		// _, _, err := DoInsertArticle(ik.NewID(ik.IDFollowing, from).String(), "following_slot", model.Article{
		// 	ID:         followID,
		// 	Cmd:        model.CmdFollow,
		// 	Extras:     map[string]string{to: state},
		// 	CreateTime: time.Now(),
		// })
		// return err
	}

	DoUpdateArticle(ik.NewID(ik.IDFollowing, from).String(), func(a *model.Article) {
		a.Extras[lastElemInCompID(followID)] = "1"
	})

	if !strings.HasPrefix(oldValue, strconv.FormatBool(following)) {
		updated = true
	}
	return nil
}

func notifyNewFollower(from, to string, following bool) (E error) {
	updated, _, err := DoUpdateOrInsertCmdArticle(
		ik.NewID(ik.IDFollower, to).String(),
		makeFollowedID(to, from),
		model.CmdFollowed,
		strconv.FormatBool(following),
		from)
	if err != nil {
		return err
	}
	if updated {
		_, err = DoUpdateUser(to, func(u *model.User) { u.Followers += int32(common.BoolInt2(following)) })
	}
	return err
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
	_, _, err := DoUpdateOrInsertCmdArticle(
		ik.NewID(ik.IDBlacklist, from).String(),
		makeBlockID(from, to),
		model.CmdBlock,
		strconv.FormatBool(blocking),
		to)
	return err
}

func AcceptUser(from, to string, accept bool) (E error) {
	id := makeFollowerAcceptanceID(from, to)
	if accept {
		go func() {
			DoInsertArticle(ik.NewID(ik.IDInbox, to).String(), false, model.Article{
				Cmd:    model.CmdInboxFwAccepted,
				Extras: map[string]string{"from": from},
			})
			IncUnread(to)
		}()
	}
	return m.db.Set(id, (&model.Article{
		ID:     id,
		Extras: map[string]string{"accept": strconv.FormatBool(accept)},
	}).Marshal())
}

func LikeArticle(u *model.User, to string, liking bool) (E error) {
	from := u.ID
	updated, inserted, err := DoUpdateOrInsertCmdArticle(
		ik.NewID(ik.IDLike, from).String(),
		makeLikeID(from, to),
		model.CmdLike,
		strconv.FormatBool(liking),
		to)
	if err != nil {
		return err
	}
	if updated {
		go func() {
			a, err := DoUpdateArticle(to, func(a *model.Article) { a.Likes += int32(common.BoolInt2(liking)) })
			if err != nil {
				log.Println("LikeArticle error:", err)
				return
			}

			if inserted && liking {
				if u.HideLikes == 0 {
					// Insert an article into from's timeline
					DoInsertArticle(ik.NewID(ik.IDAuthor, from).String(), false, model.Article{
						Cmd:    model.CmdTimelineLike,
						Extras: map[string]string{"from": from, "article_id": to},
					})
				}

				if IsFollowing(a.Author, from) {
					// If author followed 'from', notify the author that his articles has been liked by 'from'
					DoInsertArticle(ik.NewID(ik.IDInbox, a.Author).String(), false, model.Article{
						Cmd:    model.CmdInboxLike,
						Extras: map[string]string{"from": from, "article_id": to},
					})
					IncUnread(a.Author)
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
		flags = bits.Unpack256(parts[1])
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
		cursor = strconv.Itoa(idx) + "~" + bits.Pack256(flags)
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
	if to.FollowApply == 0 {
		// 'to' didn't have the switch on, so no acceptance needed for 'from'
		return IsFollowing(from, to.ID), true
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
	if time.Unix(ts, 0).Before(to.FollowApplyPivotTime()) {
		return true, true // 'from' followed 'to' before 'to' turned on the switch, so 'from' gains acceptance automatically
	}
	accept, _ := GetArticle(makeFollowerAcceptanceID(to.ID, from))
	if accept == nil {
		return true, false
	}
	return true, accept.Extras["accept"] == "true"
}

func IsBlocking(from, to string) bool {
	p, _ := WeakGetArticle(makeBlockID(from, to))
	return p != nil && p.Extras["block"] == "true"
}

func IsLiking(from, to string) bool {
	p, _ := WeakGetArticle(makeLikeID(from, to))
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

func CacheGet(key string) (string, bool) {
	v, ok := m.activeUsers.Get(key)
	return string(v), ok
}

func CacheSet(key string, value string) {
	m.activeUsers.Add(key, []byte(value))
}

func IncUnread(id string) error {
	_, err := DoUpdateUser(id, func(u *model.User) { u.Unread++ })
	return err
}
