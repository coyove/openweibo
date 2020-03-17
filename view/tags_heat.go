package view

import (
	"sort"
	"sync"
	"time"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/dal/forgettable/goforget"
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
	tagHeatCache goforget.Distribution
)

func TagHeat(g *gin.Context) []HotTag {
	tagHeatOnce.Do(func() {
		go func() {
			for {
				tagHeatCache = goforget.TopN("tagheat", 5)
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

	tags := make([]HotTag, len(res.Data))

	for k, r := range res.Data {
		x := &tags[i]
		a, _ := dal.GetArticle(ik.NewID(ik.IDTag, k).String())
		if a != nil {
			var av ArticleView
			av.from(a, 0, u)
			x.Tag = av
			x.LastUpdated = ik.ParseID(a.NextID).Time()
		}
		x.IsFollowing = dal.IsFollowing(u.ID, "#"+k)
		x.Name, x.FullName = k, "#"+k
		x.Score = r.P
		x.Count = r.Count
		i++
	}

	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Score > tags[j].Score
	})

	return tags
}
