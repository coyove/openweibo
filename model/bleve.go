// TODO
package model

import (
	"time"

	"github.com/coyove/common/lru"
)

var (
	bleveFastCache *lru.Cache
	bleveWorker    chan *Article
)

func init() {
	bleveFastCache = lru.NewCache(10240)
	bleveWorker = make(chan *Article, 1024)
	go indexArticleWorker()
}

func indexArticle(a *Article) {
	if a.PostOptions&PostOptionNoMasterTimeline != 0 {
		return
	}
	if a.PostOptions&PostOptionNoSearch != 0 {
		return
	}
	if !(len(a.ID) == 12 && a.ID[0] == 'S') {
		return
	}

	select {
	case bleveWorker <- a:
	default:
	}
}

func indexArticleWorker() {
	for a := range bleveWorker {
		if _, ok := bleveFastCache.Get(a.ID); ok {
			continue
		}
		Index("art", a.ID, a.Content, a.Author)
		bleveFastCache.Add(a.ID, true)
	}
}

func SearchMetrics() string {
	return ""
}

func SearchArticle(q string, timeout time.Duration, start, limit int) ([]string, int, error) {
	return Search("art", q, start, limit)
}
