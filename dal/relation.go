package dal

import (
	"log"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/iis/common"
	"github.com/coyove/iis/ik"
	"github.com/coyove/iis/model"
)

type getCommonFollowingListTask struct {
	from, to string
	cursor   string
	n        int
}

var deferredGetCommonFollowingList chan getCommonFollowingListTask

func init() {
	deferredGetCommonFollowingList = make(chan getCommonFollowingListTask, 1024)
	go func() {
		for {
			select {
			case t := <-deferredGetCommonFollowingList:
				GetCommonFollowingList(t.from, t.to, t.cursor, t.n)
			}
			time.Sleep(time.Millisecond * 500)
		}
	}()
}

func GetCommonFollowingList(from, to string, cursor string, n int) ([]FollowingState, string) {
	var idx int
	var flags map[string]string

	if parts := strings.Split(cursor, "~"); len(parts) != 2 {
		// Start from the root article
		master, err := GetArticle(ik.NewID(ik.IDFollowing, from).String())
		if err != nil {
			if err != model.ErrNotExisted {
				log.Println("[GetCommonFollowingList] Failed to get chain [", from, "]")
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
	timedout := false

	for ; len(res) < n && idx < 256; idx++ {
		if flags[strconv.Itoa(idx)] != "1" {
			continue
		}

		if time.Since(start).Seconds() > 0.2+rand.Float64()/10 {
			timedout = true
			break
		}

		a, err := GetArticle("u/" + from + "/follow/" + strconv.Itoa(idx))
		if err != nil {
			log.Println("[GetCommonFollowingList]", cursor, err)
			break
		}

		var a2Extras map[string]string
		a2, err := GetArticle("u/" + to + "/follow/" + strconv.Itoa(idx))
		if err == nil {
			a2Extras = a2.Extras
		}

		keys := []string{}
		for k, v := range a.Extras {
			if !strings.HasPrefix(v, "true,") { // 'from' doesn't follow 'k'
				continue
			}
			keys = append(keys, makeFollowID(k, to)) // let's find out whether every user who is followed by 'from' has followed 'to'
		}
		twoHopsFollowings := MGetArticlesFromCache(keys...)

		for k, v := range a.Extras {
			if !strings.HasPrefix(v, "true,") { // 'from' doesn't follow 'k'
				continue
			}
			toFollowK := strings.HasPrefix(a2Extras[k], "true,") // 'to' follow 'k'

			p := strings.Split(v, ",")
			s := FollowingState{
				ID:              k,
				Time:            time.Unix(atoi64(p[1]), 0),
				CommonFollowing: toFollowK,
			}

			if key2 := makeFollowID(k, to); twoHopsFollowings[key2] != nil {
				two := twoHopsFollowings[key2]
				if strings.HasPrefix(two.Extras[to], "true,") {
					s.TwoHopsFollowing = true
				}
			}

			if s.CommonFollowing || s.TwoHopsFollowing {
				if !strings.HasPrefix(k, "#") {
					s.FullUser, _ = GetUser(k)
				} else {
					s.FullUser = &model.User{ID: k}
				}
				if s.FullUser != nil {
					res = append(res, s)
				}
			}
		}
	}

	sort.Slice(res, func(i, j int) bool { return res[i].Time.After(res[j].Time) })

	if idx > 255 {
		cursor = ""
	} else {
		cursor = strconv.Itoa(idx) + "~" + common.Pack256(flags)
	}

	if timedout && cursor != "" {
		// Continue calling the function to preload the data into cache
		select {
		case deferredGetCommonFollowingList <- getCommonFollowingListTask{from, to, cursor, n}:
		default:
		}
	}

	return res, cursor
}
