package handler

import (
	"sync"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/dal/tagrank"
	"github.com/coyove/iis/ik"
	"github.com/gin-gonic/gin"
)

type HotTag struct {
	Tag         ArticleView
	Name        string
	FullName    string
	LastUpdated time.Time
	Count       int
	Score       float64
	IsFollowing bool
}

var (
	tagHeatOnce  sync.Once
	tagHeatCache []string
)

func TagHeat(g *gin.Context) []HotTag {
	tagHeatOnce.Do(func() {
		go func() {
			for {
				tagHeatCache = tagrank.TopN(5)
				time.Sleep(time.Second * 5)
			}
		}()
	})

	i, u := 0, getUser(g)
	if u == nil {
		return nil
	}

	res := tagHeatCache
	// res.Data["a"] = &goforget.CmdValue{Count: 10, P: 100}

	tags := make([]HotTag, len(res))

	for _, k := range res {
		x := &tags[i]
		a, _ := dal.WeakGetArticle(ik.NewID(ik.IDTag, k).String())
		if a != nil {
			var av ArticleView
			av.from(a, 0, u)
			x.Tag = av
			x.LastUpdated = ik.ParseID(a.NextID).Time()
			x.Count = a.Replies
		}
		x.IsFollowing = dal.IsFollowing(u.ID, "#"+k)
		x.Name, x.FullName = k, "#"+k
		i++
	}

	return tags
}
