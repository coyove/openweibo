package mv

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/coyove/common/lru"
	"github.com/coyove/iis/cmd/ch/config"
)

var (
	db       bleve.Index
	docCount uint64
	q        chan *Article
	cache    *lru.Cache
)

func InitSearchIndex(path string) {
	_, err := os.Stat(path)
	if err != nil {
		mapping := bleve.NewIndexMapping()
		db, err = bleve.New(path, mapping)
	} else {
		db, err = bleve.Open(path)
	}

	if err != nil {
		panic(err)
	}

	q = make(chan *Article, 1024)
	cache = lru.NewCache(65536)

	go func() {
		for a := range q {
			if _, ok := cache.Get(a.ID); ok {
				continue
			}
			if doc, _ := db.Document(a.ID); doc != nil {
				cache.Add(a.ID, true)
				continue
			}
			if err := db.Index(a.ID, a.Title+"\ueeee"+a.Content+"\ueeee"+a.Author); err != nil {
				log.Println("Search index failed:", err)
				continue
			}

			docCount, _ = db.DocCount()
			log.Println("Search index:", a.ID, "count:", docCount)
			cache.Add(a.ID, true)
		}
	}()
}

func Search(query string) []string {
	q := bleve.NewQueryStringQuery(query)

	req := bleve.NewSearchRequest(q)
	req.Size = config.Cfg.PostsPerPage

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	searchResult, err := bleve.MultiSearch(ctx, req, db)
	if err != nil {
		log.Println("Search failed:", err, "query:", query)
		return nil
	}

	ret := []string{}
	for _, m := range searchResult.Hits {
		ret = append(ret, m.ID)
	}
	return ret
}

func BuildIndex(a *Article) {
	select {
	case q <- a:
	default:
	}
}

func SearchStat() (map[string]interface{}, int) {
	m := db.StatsMap()
	return m, int(docCount)
}
