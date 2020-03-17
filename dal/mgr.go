package dal

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/coyove/iis/common"
	"github.com/coyove/iis/dal/kv"
	"github.com/coyove/iis/dal/kv/cache"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
)

var m struct {
	db          KeyValueOp
	activeUsers *cache.GlobalCache
}

func Init(redisConfig *cache.RedisConfig, region string, ak, sk string) {
	const CacheSize int64 = 10000

	var db KeyValueOp

	if region == "" {
		db = kv.NewDiskKV()
	} else {
		db = kv.NewDynamoKV(region, ak, sk)
	}

	c := cache.NewGlobalCache(CacheSize, redisConfig)
	db.SetGlobalCache(c)

	m.db = db
	m.activeUsers = c
}

func ModKV() KeyValueOp {
	return m.db
}

func MGetArticlesFromCache(keys ...string) map[string]*model.Article {
	if len(keys) > 1024 {
		keys = keys[:1024]
	}
	res := m.activeUsers.MGet(keys...)
	m := map[string]*model.Article{}
	for k, v := range res {
		a, err := model.UnmarshalArticle(v)
		if err == nil {
			m[k] = a
		}
	}
	return m
}

func GetArticle(id string, dontOverrideNextID ...bool) (*model.Article, error) {
	if id == "" {
		return nil, fmt.Errorf("empty ArticleID")
	}
	p, err := m.db.Get(id)
	if err != nil {
		return nil, err
	}
	if len(p) == 0 {
		return nil, model.ErrNotExisted
	}
	a, err := model.UnmarshalArticle(p)
	if err != nil {
		return nil, err
	}
	if a.ReferID == "" {
		return a, nil
	}
	a2, err := GetArticle(a.ReferID)
	if err != nil {
		return nil, err
	}
	if len(dontOverrideNextID) == 1 && dontOverrideNextID[0] {
		return a2, nil
	}
	a2.NextID = a.NextID
	a2.NextMediaID = a.NextMediaID
	return a2, nil
}

func WalkMulti(media bool, n int, cursors ...ik.ID) (a []*model.Article, next []ik.ID) {
	if len(cursors) == 0 {
		return
	}

	// Quick hack: mget from cache first
	trykeys, trykeysIndex := []string{}, []int{}
	for i, c := range cursors {
		if hdr := c.Header(); hdr == ik.IDAuthor || hdr == ik.IDTag {
			trykeys = append(trykeys, c.String())
			trykeysIndex = append(trykeysIndex, i)
		}
	}
	if len(trykeys) > 0 {
		m := MGetArticlesFromCache(trykeys...)
		count := 0
		for i, k := range trykeys {
			if m[k] != nil {
				cursors[trykeysIndex[i]] = ik.ParseID(m[k].PickNextID(media))
				count++
			}
		}
	}

	startTime := time.Now()
	idm := map[string]bool{}
	idmp := map[string]bool{} // dedup map for parent articles

	for len(a) < n {
		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.WalkMulti] Break out slow walk at", cursors)
			break
		}

	DEDUP: // not some very good dedup code
		dedup := make(map[ik.ID]bool, len(cursors))
		for i, c := range cursors {
			if dedup[c] {
				cursors = append(cursors[:i], cursors[i+1:]...)
				goto DEDUP
			}
			dedup[c] = true
		}

		sort.Slice(cursors, func(i, j int) bool {
			if ii, jj := cursors[i].Time(), cursors[j].Time(); ii == jj {
				return bytes.Compare(cursors[i].TagBytes(), cursors[j].TagBytes()) < 0
			} else if cursors[i].IsRoot() { // i is bigger than j
				return false
			} else if cursors[j].IsRoot() {
				return true
			} else {
				return ii.Before(jj)
			}
		})

		latest := &cursors[len(cursors)-1]
		if !latest.Valid() {
			break
		}

		p, err := GetArticle(latest.String())
		if err == nil {
			ok := !idm[p.ID] && p.Content != model.DeletionMarker && !latest.IsRoot()
			// 1. 'p' is not duplicated
			// 2. 'p' is not deleted
			// 3. 'p' is not a root article

			if p.Parent == "" && idmp[p.ID] {
				// 4. if 'p' is a top article and has been replied before (presented in 'idmp')
				//    ignore it to clean the timeline a bit
				ok = false
			}

			if ok {
				a = append(a, p)

				idm[p.ID] = true
				if p.Parent != "" {
					idmp[p.Parent] = true
				}
			}
			*latest = ik.ParseID(p.PickNextID(media))
		} else {
			if err != model.ErrNotExisted {
				log.Println("[mgr.WalkMulti] Failed to get:", latest.String(), err)
			}

			*latest = ik.ID{}
		}
	}

	return a, cursors
}

func WalkReply(n int, cursor string) (a []*model.Article, next string) {
	startTime := time.Now()

	for len(a) < n && cursor != "" {
		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.WalkReply] Break out slow walk at", cursor)
			break
		}

		p, err := GetArticle(cursor)
		if err != nil {
			log.Println("[mgr.WalkReply] Failed to get:", cursor, err)
			break
		}

		if p.Content != model.DeletionMarker {
			a = append(a, p)
		}
		cursor = p.NextReplyID
	}

	return a, cursor
}

func WalkLikes(media bool, n int, cursor string) (a []*model.Article, next string) {
	startTime := time.Now()

	for len(a) < n && cursor != "" {
		if time.Since(startTime).Seconds() > 1 {
			log.Println("[mgr.WalkLikes] Break out slow walk at", cursor)
			break
		}

		p, err := GetArticle(cursor)
		if err != nil {
			log.Println("[mgr.WalkLikes] Failed to get:", cursor, err)
			break
		}

		if p.Extras["like"] == "true" {
			a2, err := GetArticle(p.Extras["to"])
			if err == nil {
				a2.NextID = p.NextID
				a = append(a, a2)
			} else {
				log.Println("[mgr.WalkLikes] Failed to get:", p.Extras["to"], err)
			}
		}

		cursor = p.PickNextID(media)
	}

	return a, cursor
}

func Post(a *model.Article, author *model.User, noMaster bool) (*model.Article, error) {
	a.ID = ik.NewGeneralID().String()
	a.CreateTime = time.Now()
	a.Author = author.ID

	if _, err := DoInsertArticle(&InsertArticleRequest{
		ID:      ik.NewID(ik.IDAuthor, a.Author).String(),
		Article: *a,
	}); err != nil {
		return nil, err
	}

	go func() {
		if !noMaster {
			DoInsertArticle(&InsertArticleRequest{
				ID: ik.NewID(ik.IDAuthor, "master").String(),
				Article: model.Article{
					ID:         ik.NewGeneralID().String(),
					ReferID:    a.ID,
					Media:      a.Media,
					CreateTime: time.Now(),
				},
			})
		}
		ids, tags := common.ExtractMentionsAndTags(a.Content)
		MentionUserAndTags(a, ids, tags)
	}()

	return a, nil
}

func PostReply(parent string, content, media string, author *model.User, ip string, nsfw bool, noTimeline bool) (*model.Article, error) {
	p, err := GetArticle(parent)
	if err != nil {
		return nil, err
	}

	if p.Locked && p.Author != author.ID { // The author himself can reply to his own locked articles
		if !author.IsMod() {
			return nil, fmt.Errorf("locked parent")
		}
	}

	if IsBlocking(p.Author, author.ID) {
		if !author.IsMod() {
			return nil, fmt.Errorf("author blocked")
		}
	}

	a := &model.Article{
		ID:         ik.NewGeneralID().String(),
		Content:    content,
		Media:      media,
		NSFW:       nsfw,
		Author:     author.ID,
		IP:         ip,
		Parent:     p.ID,
		CreateTime: time.Now(),
	}

	a2, err := DoInsertArticle(&InsertArticleRequest{ID: p.ID, Article: *a, AsReply: true})
	if err != nil {
		return nil, err
	}
	a = &a2

	if !noTimeline {
		// Add reply to its timeline
		if _, err := DoInsertArticle(&InsertArticleRequest{
			ID:      ik.NewID(ik.IDAuthor, a.Author).String(),
			Article: *a,
		}); err != nil {
			return nil, err
		}
	}

	go func() {
		if p.Content != model.DeletionMarker && a.Author != p.Author {
			if _, err := DoInsertArticle(&InsertArticleRequest{
				ID: ik.NewID(ik.IDInbox, p.Author).String(),
				Article: model.Article{
					ID:  ik.NewGeneralID().String(),
					Cmd: model.CmdReply,
					Extras: map[string]string{
						"from":       a.Author,
						"article_id": a.ID,
					},
					CreateTime: time.Now(),
				},
			}); err != nil {
				log.Println("PostReply", err)
			}

			DoUpdateUser(&UpdateUserRequest{ID: p.Author, IncDecUnread: aws.Bool(true)})
		}
		ids, tags := common.ExtractMentionsAndTags(a.Content)
		MentionUserAndTags(a, ids, tags)
	}()

	return a, nil
}
