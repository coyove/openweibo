// TODO
package model

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/coyove/common/lru"
)

var (
	bleveIndex     bleve.Index
	bleveFastCache *lru.Cache
	bleveWorker    chan *Article
	blevePath      string
	BleveIndexed   int
)

func OpenBleve(path string) {
	mapping := bleve.NewIndexMapping()
	index, err := bleve.New(path, mapping)
	if err != nil {
		index, err = bleve.Open(path)
		if err != nil {
			panic(err)
		}
	}
	bleveIndex = index
	bleveWorker = make(chan *Article, 1024)
	bleveFastCache = lru.NewCache(1024)
	blevePath = path
	go indexArticleWorker()
}

func indexArticle(a *Article) {
	if a.PostOptions&PostOptionNoMasterTimeline != 0 {
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
	w := func(a *Article) {
		if _, ok := bleveFastCache.Get(a.ID); ok {
			return
		}
		if doc, _ := bleveIndex.Document(a.ID); doc != nil {
			return
		}

		if err := bleveIndex.Index(a.ID, a); err != nil {
			log.Println("[Bleve] index error:", err)
			return
		}
		bleveFastCache.Add(a.ID, true)
	}

	for a := range bleveWorker {
		count, _ := bleveIndex.DocCount()
		w(a)
		binfo, _ := os.Stat(blevePath)
		if binfo != nil {
			mb := binfo.Size() / 1024 / 1024
			count = count*1e6 + uint64(mb)
		}
		BleveIndexed = int(count)
	}
}

func SearchMetrics() string {
	p, _ := bleveIndex.Stats().MarshalJSON()
	return string(p)
}

func SearchArticle(q string, timeout time.Duration, start, limit int) ([]string, int, error) {
	query := bleve.NewMatchQuery(q)
	req := bleve.NewSearchRequest(query)
	req.From = start
	req.Size = limit
	req.SortBy([]string{"-_id"})

	if bleve.MemoryNeededForSearchResult(req) > 1024*1024 {
		return nil, 0, fmt.Errorf("complex query")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res, err := bleveIndex.SearchInContext(ctx, req)
	if err != nil {
		return nil, 0, err
	}

	ids := []string{}
	for i := range res.Hits {
		ids = append(ids, res.Hits[i].ID)
	}
	return ids, int(res.Total), nil
}
