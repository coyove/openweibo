package main

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
	"go.etcd.io/bbolt"
)

func downloadData() {
	downloadWiki := func(p string) ([]string, error) {
		req, _ := http.NewRequest("GET", "https://dumps.wikimedia.org/zhwiki/20230101/"+p, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		rd := bufio.NewReader(bzip2.NewReader(resp.Body))

		var res []string
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					return nil, err
				}
				break
			}
			parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
			x := parts[2]
			if strings.HasPrefix(x, "Category:") ||
				strings.HasPrefix(x, "WikiProject:") ||
				strings.HasPrefix(x, "Wikipedia:") ||
				strings.HasPrefix(x, "File:") ||
				strings.HasPrefix(x, "Template:") {
				continue
			}
			res = append(res, x)
		}
		return res, nil
	}

	for i, p := range strings.Split(`zhwiki-20230101-pages-articles-multistream-index1.txt-p1p187712.bz2
	zhwiki-20230101-pages-articles-multistream-index2.txt-p187713p630160.bz2
	zhwiki-20230101-pages-articles-multistream-index3.txt-p630161p1389648.bz2
	zhwiki-20230101-pages-articles-multistream-index4.txt-p1389649p2889648.bz2
	zhwiki-20230101-pages-articles-multistream-index4.txt-p2889649p3391029.bz2
	zhwiki-20230101-pages-articles-multistream-index5.txt-p3391030p4891029.bz2
	zhwiki-20230101-pages-articles-multistream-index5.txt-p4891030p5596379.bz2
	zhwiki-20230101-pages-articles-multistream-index6.txt-p5596380p7096379.bz2
	zhwiki-20230101-pages-articles-multistream-index6.txt-p7096380p8231694.bz2`, "\n") {
		v, err := downloadWiki(p)
		fmt.Println(p, len(v), err)

		buf := strings.Join(v, "\n")
		ioutil.WriteFile("data"+strconv.Itoa(i), []byte(buf), 0777)
	}

	f, _ := os.Open("out")
	rd := bufio.NewReader(f)
	data := map[string]bool{}
	for i := 0; ; i++ {
		line, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		if data[line] {
			continue
		}
		data[line] = true
	}
	f.Close()
	f, _ = os.Create("out2")
	for k := range data {
		f.WriteString(k)
	}
	f.Close()
}

func rebuildData(count int) {
	data := map[uint64]string{}
	mgr := dal.Store.Manager
	f, _ := os.Open("out.gz")
	gr, _ := gzip.NewReader(f)
	rd := bufio.NewReader(gr)
	for i := 0; count <= 0 || i < count; i++ {
		line, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		line = html.UnescapeString(strings.TrimSpace(line))
		h := buildBitmapHashes(line, "bulk", nil)
		if len(h) == 0 {
			continue
		}
		id := clock.Id()
		data[id] = line
		k := bitmap.Uint64Key(id)
		mgr.Saver().AddAsync(k, h)
		if i%100000 == 0 {
			log.Println(i)
		}
	}
	mgr.Saver().Close()

	for len(data) > 0 {
		dal.Store.DB.Update(func(tx *bbolt.Tx) error {
			c := 0
			for i, line := range data {
				k := types.Uint64Bytes(uint64(i))
				now := clock.UnixMilli()
				ksv := dal.KeySortValue{
					Key:   k[:],
					Sort0: uint64(now),
					Sort1: []byte(line),
					Value: (&types.Note{
						Id:         uint64(i),
						Title:      line,
						Creator:    "bulk",
						CreateUnix: now,
						UpdateUnix: now,
					}).MarshalBinary(),
				}
				dal.KSVUpsert(tx, "notes", ksv)
				dal.KSVUpsert(tx, "creator_bulk", dal.KeySortValue{
					Key:   k[:],
					Sort0: uint64(now),
					Sort1: []byte(line),
				})

				delete(data, i)
				c++
				if c > 1000 {
					break
				}
			}
			return nil
		})
		fmt.Println(len(data))
	}
}
